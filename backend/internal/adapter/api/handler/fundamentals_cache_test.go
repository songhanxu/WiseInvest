package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type snapshotFixture struct {
	row FundamentalsResponse
	at  time.Time
}

func (s snapshotFixture) Load(context.Context, string) (FundamentalsResponse, time.Time, bool, error) {
	return s.row, s.at, true, nil
}
func (snapshotFixture) Save(context.Context, string, FundamentalsResponse, time.Time) error {
	return nil
}
func (snapshotFixture) WatchedCodes(context.Context) ([]string, error) { return nil, nil }

func TestSECSnapshotServedAfterServiceRestart(t *testing.T) {
	eps := 4.25
	s := &FundamentalsService{
		SEC:   &secFundamentalsClient{},
		store: snapshotFixture{row: FundamentalsResponse{Status: "available", EPS: &eps}, at: time.Now()},
	}
	row := s.secResult(context.Background(), "TEST")
	if row.EPS == nil || *row.EPS != eps {
		t.Fatalf("persisted SEC value was not served: %+v", row)
	}
}

func TestSourceCacheReturnsStaleWhileSingleRefreshRuns(t *testing.T) {
	s := &FundamentalsService{
		cache:         make(map[string]fundamentalsCache),
		sourceCache:   make(map[string]sourceCacheEntry),
		sourcePending: make(map[string]chan struct{}),
	}
	value := 1.0
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) FundamentalsResponse {
		if calls.Add(1) == 1 {
			return FundamentalsResponse{Status: "available", EPS: &value}
		}
		close(started)
		<-release
		updated := 2.0
		return FundamentalsResponse{Status: "available", EPS: &updated}
	}
	ctx := context.Background()
	if row := s.sourceResult(ctx, "sec:TEST", "us_stock:TEST", time.Hour, fetch); row.EPS == nil || *row.EPS != 1 {
		t.Fatalf("initial fetch = %+v", row)
	}
	s.mu.Lock()
	entry := s.sourceCache["sec:TEST"]
	entry.NextRefresh = time.Now().Add(-time.Second)
	s.sourceCache["sec:TEST"] = entry
	s.mu.Unlock()
	if row := s.sourceResult(ctx, "sec:TEST", "us_stock:TEST", time.Hour, fetch); row.EPS == nil || *row.EPS != 1 {
		t.Fatalf("expected previous value during refresh, got %+v", row)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	for i := 0; i < 4; i++ {
		row := s.sourceResult(ctx, "sec:TEST", "us_stock:TEST", time.Hour, fetch)
		if row.EPS == nil || *row.EPS != 1 {
			t.Fatalf("stale request %d = %+v", i, row)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected one background refresh, got %d fetches", got)
	}
	close(release)
	deadline := time.After(time.Second)
	for {
		row := s.sourceResult(ctx, "sec:TEST", "us_stock:TEST", time.Hour, fetch)
		if row.EPS != nil && *row.EPS == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("refreshed value was not published")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestFailedSourceUsesBackoff(t *testing.T) {
	s := &FundamentalsService{}
	var calls atomic.Int32
	fetch := func(context.Context) FundamentalsResponse {
		calls.Add(1)
		return FundamentalsResponse{Status: "unavailable", Message: "upstream failed"}
	}
	for i := 0; i < 3; i++ {
		s.sourceResult(context.Background(), "yahoo:TEST", "us_stock:TEST", time.Hour, fetch)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("failed source retried %d times without backoff", got)
	}
}
