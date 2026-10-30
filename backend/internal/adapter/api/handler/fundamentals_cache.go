package handler

import (
	"context"
	"strings"
	"time"
)

type sourceCacheEntry struct {
	Data        FundamentalsResponse
	NextRefresh time.Time
	Stale       bool
}

// sourceResult returns the previous successful response immediately after its
// freshness window and starts exactly one refresh in the background.
func (s *FundamentalsService) sourceResult(ctx context.Context, key, aggregateKey string, ttl time.Duration, fetch func(context.Context) FundamentalsResponse) FundamentalsResponse {
	for {
		s.mu.Lock()
		if s.sourceCache == nil {
			s.sourceCache = make(map[string]sourceCacheEntry)
		}
		if s.sourcePending == nil {
			s.sourcePending = make(map[string]chan struct{})
		}
		entry, exists := s.sourceCache[key]
		pending := s.sourcePending[key]
		if exists {
			if time.Now().After(entry.NextRefresh) && pending == nil {
				done := make(chan struct{})
				s.sourcePending[key] = done
				go func() {
					refreshCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
					defer cancel()
					s.refreshSource(refreshCtx, key, aggregateKey, ttl, fetch, done)
				}()
			}
			s.mu.Unlock()
			if (entry.Stale || time.Now().After(entry.NextRefresh)) && hasFundamentalData(entry.Data) {
				entry.Data.Message = strings.Trim(strings.Join([]string{entry.Data.Message, "显示上次取得的数据，后台正在更新"}, "；"), "；")
			}
			return entry.Data
		}
		if pending == nil {
			pending = make(chan struct{})
			s.sourcePending[key] = pending
			s.mu.Unlock()
			s.refreshSource(ctx, key, aggregateKey, ttl, fetch, pending)
			continue
		}
		s.mu.Unlock()
		select {
		case <-pending:
		case <-ctx.Done():
			return FundamentalsResponse{Status: "unavailable", Message: "数据源查询超时"}
		}
	}
}

func (s *FundamentalsService) refreshSource(ctx context.Context, key, aggregateKey string, ttl time.Duration, fetch func(context.Context) FundamentalsResponse, done chan struct{}) {
	row := fetch(ctx)
	success := row.Status == "not_applicable" || hasFundamentalData(row)
	next := time.Now().Add(ttl)
	if !success {
		next = time.Now().Add(10 * time.Minute)
	}
	s.mu.Lock()
	previous, hadPrevious := s.sourceCache[key]
	stale := false
	if !success && hadPrevious && hasFundamentalData(previous.Data) {
		row = previous.Data
		stale = true
	}
	if s.sourceCache == nil {
		s.sourceCache = make(map[string]sourceCacheEntry)
	}
	if len(s.sourceCache) >= 2048 {
		oldestKey := ""
		var oldest time.Time
		for candidate, entry := range s.sourceCache {
			if oldestKey == "" || entry.NextRefresh.Before(oldest) {
				oldestKey, oldest = candidate, entry.NextRefresh
			}
		}
		delete(s.sourceCache, oldestKey)
	}
	s.sourceCache[key] = sourceCacheEntry{Data: row, NextRefresh: next, Stale: stale}
	delete(s.cache, aggregateKey)
	delete(s.sourcePending, key)
	close(done)
	s.mu.Unlock()
}

func (s *FundamentalsService) secResult(ctx context.Context, code string) FundamentalsResponse {
	key := "sec:us_stock:" + code
	s.mu.Lock()
	_, exists := s.sourceCache[key]
	store := s.store
	s.mu.Unlock()
	if !exists && store != nil {
		if row, at, found, err := store.Load(ctx, code); err == nil && found && hasFundamentalData(row) {
			s.mu.Lock()
			if s.sourceCache == nil {
				s.sourceCache = make(map[string]sourceCacheEntry)
			}
			if _, present := s.sourceCache[key]; !present {
				s.sourceCache[key] = sourceCacheEntry{Data: row, NextRefresh: at.Add(30 * time.Minute)}
			}
			s.mu.Unlock()
		}
	}
	return s.sourceResult(ctx, key, "us_stock:"+code, 30*time.Minute, func(fetchCtx context.Context) FundamentalsResponse {
		row := s.SEC.fetch(fetchCtx, code)
		if store != nil && hasFundamentalData(row) {
			saveCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = store.Save(saveCtx, code, row, time.Now())
			cancel()
		}
		return row
	})
}
