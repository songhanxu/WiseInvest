package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

type memoryHoldingsSnapshotStore struct {
	mu       sync.Mutex
	snapshot HoldingsSnapshot
	found    bool
	saves    int
}

func (s *memoryHoldingsSnapshotStore) Load(context.Context, string, uint) (HoldingsSnapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneHoldingsSnapshot(s.snapshot), s.found, nil
}

func (s *memoryHoldingsSnapshotStore) Save(_ context.Context, _ string, _ uint, snapshot HoldingsSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = cloneHoldingsSnapshot(snapshot)
	s.found = true
	s.saves++
	return nil
}

func TestHoldingsSnapshotSurvivesRestartAndUpstreamFailure(t *testing.T) {
	store := &memoryHoldingsSnapshotStore{}
	fresh := HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{HoldingID: "spot:rAAPL", Quantity: "2"}}, AsOf: "2026-10-31T06:05:30Z", Complete: true, Warnings: []string{}}
	first := &AccountHandler{cfg: config.BitgetConfig{OwnerUserID: 7}, loader: func(context.Context) (HoldingsSnapshot, error) { return fresh, nil }}
	if err := first.configureSnapshotStore(context.Background(), store, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := first.holdings(context.Background()); err != nil || got.Stale || store.saves != 1 {
		t.Fatalf("fresh snapshot was not persisted: %+v err=%v saves=%d", got, err, store.saves)
	}

	restarted := &AccountHandler{cfg: config.BitgetConfig{OwnerUserID: 7}, loader: func(context.Context) (HoldingsSnapshot, error) {
		return HoldingsSnapshot{Warnings: []string{"upstream unavailable"}}, errors.New("offline")
	}}
	if err := restarted.configureSnapshotStore(context.Background(), store, ""); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.holdings(context.Background())
	if err != nil || !got.Stale || got.Complete || got.LastSuccess != fresh.AsOf || len(got.Holdings) != 1 || got.Holdings[0].Quantity != "2" {
		t.Fatalf("durable fallback lost after restart: %+v err=%v", got, err)
	}
	if store.saves != 1 {
		t.Fatalf("failed response overwrote durable snapshot: saves=%d", store.saves)
	}
}

func TestPartialHoldingsNeverOverwriteDurableSnapshot(t *testing.T) {
	store := &memoryHoldingsSnapshotStore{snapshot: HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{Quantity: "old"}}, AsOf: "2026-01-01T00:00:00Z", LastSuccess: "2026-01-01T00:00:00Z", Complete: true}, found: true}
	h := &AccountHandler{cfg: config.BitgetConfig{OwnerUserID: 3}, loader: func(context.Context) (HoldingsSnapshot, error) {
		return HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{Quantity: "partial"}}, AsOf: time.Now().UTC().Format(time.RFC3339), Complete: false, Warnings: []string{"partial"}}, nil
	}}
	if err := h.configureSnapshotStore(context.Background(), store, ""); err != nil {
		t.Fatal(err)
	}
	got, err := h.holdings(context.Background())
	if err != nil || !got.Stale || got.Holdings[0].Quantity != "old" || store.saves != 0 {
		t.Fatalf("partial response replaced snapshot: %+v err=%v saves=%d", got, err, store.saves)
	}
}

func TestPrivateSeedImportsOnlyWhenStoreIsEmpty(t *testing.T) {
	store := &memoryHoldingsSnapshotStore{}
	file := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(file, []byte(`{"account_id":"bitget","holdings":[],"as_of":"2026-10-31T06:05:30Z","stale":false,"complete":true,"warnings":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	h := &AccountHandler{cfg: config.BitgetConfig{OwnerUserID: 9}}
	if err := h.configureSnapshotStore(context.Background(), store, file); err != nil {
		t.Fatal(err)
	}
	if store.saves != 1 || h.snapshot == nil || h.snapshot.AsOf != "2026-10-31T06:05:30Z" {
		t.Fatalf("seed not imported: saves=%d snapshot=%+v", store.saves, h.snapshot)
	}
	if err := h.configureSnapshotStore(context.Background(), store, filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatalf("existing database snapshot should bypass seed file: %v", err)
	}
}

func TestAutomationEndpointUsesDurableSnapshotFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accessKey := strings.Repeat("a", 64)
	store := &memoryHoldingsSnapshotStore{snapshot: HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{HoldingID: "spot:rAAPL", Quantity: "durable"}}, AsOf: "2026-10-31T06:05:30Z", LastSuccess: "2026-10-31T06:05:30Z", Complete: true}, found: true}
	h := &AccountHandler{cfg: config.BitgetConfig{APIKey: "key", APISecret: "secret", Passphrase: "pass", HoldingsAccessKey: accessKey, OwnerUserID: 7, AccountType: "uta"}, loader: func(context.Context) (HoldingsSnapshot, error) {
		return HoldingsSnapshot{}, errors.New("offline")
	}}
	if err := h.configureSnapshotStore(context.Background(), store, ""); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/automation/bitget/holdings", nil)
	c.Request.Header.Set("Authorization", "Bearer "+accessKey)
	h.GetAutomationBitgetHoldings(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"quantity":"durable"`) || !strings.Contains(w.Body.String(), `"stale":true`) {
		t.Fatalf("automation endpoint did not use durable fallback: %d %s", w.Code, w.Body.String())
	}
}
