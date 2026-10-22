package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

func accountForTest(provider string, owner uint, snapshot HoldingsSnapshot, load func(context.Context) (HoldingsSnapshot, error)) *AccountHandler {
	c := &config.ExchangeAccountConfig{Provider: provider, APIKey: "key", APISecret: "secret", OwnerUserID: owner, AccountName: provider + " private", AccountType: "spot"}
	return &AccountHandler{exchangeConfig: c, snapshot: &snapshot, cachedAt: time.Now(), loader: load}
}
func TestAggregateOwnershipAndSourceIdentity(t *testing.T) {
	calls := atomic.Int32{}
	base := HoldingsSnapshot{Holdings: []AccountHolding{{HoldingID: "spot:BTC", Quantity: "1", StockResponse: StockResponse{ID: "BTC", Market: "crypto"}}}, AsOf: "2026-10-16T00:00:00Z", Complete: true}
	root := &AccountHandler{cfg: config.BitgetConfig{APIKey: "key", APISecret: "secret", Passphrase: "pass", OwnerUserID: 7, AccountName: "my Bitget"}, snapshot: &base, cachedAt: time.Now(), others: map[string]*AccountHandler{}}
	root.others["binance"] = accountForTest("binance", 7, base, nil)
	root.others["gate"] = &AccountHandler{exchangeConfig: &config.ExchangeAccountConfig{Provider: "gate", APIKey: "key", APISecret: "secret", OwnerUserID: 8, AccountName: "hidden Gate account"}, loader: func(context.Context) (HoldingsSnapshot, error) { calls.Add(1); return base, nil }}
	result, err := root.allHoldings(context.Background(), 7)
	if err != nil || len(result.Holdings) != 2 || calls.Load() != 0 || len(result.Accounts) != 3 || !result.Complete {
		t.Fatalf("aggregate isolation failed: %+v %v", result, err)
	}
	if result.Holdings[0].HoldingID == result.Holdings[1].HoldingID || base.Holdings[0].AccountID != "" || base.Holdings[0].HoldingID != "spot:BTC" {
		t.Fatal("cache mutated or cross-account identity collapsed")
	}
	for _, state := range result.Accounts {
		if strings.Contains(state.Name, "hidden") || state.ID == "gate" && state.Status != "unbound" {
			t.Fatal("other user's account metadata exposed")
		}
	}
	result, err = root.allHoldings(context.Background(), 9)
	if err != nil || len(result.Holdings) != 0 || calls.Load() != 0 {
		t.Fatal("aggregate exposed another user's holdings")
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", uint(9))
	c.Params = gin.Params{{Key: "accountID", Value: "binance"}}
	c.Request = httptest.NewRequest("GET", "/accounts/binance/holdings", nil)
	root.GetHoldings(c)
	if w.Code != http.StatusForbidden {
		t.Fatal("individual account ownership was bypassed")
	}
}
func TestAggregateRetainsOtherAccountsWhenOneFails(t *testing.T) {
	root := NewAccountHandler(config.BitgetConfig{})
	snapshot := HoldingsSnapshot{Holdings: []AccountHolding{{HoldingID: "spot:BTC", Quantity: "2"}}, Complete: true, AsOf: "2026-10-16T00:00:00Z"}
	root.others = map[string]*AccountHandler{"binance": accountForTest("binance", 7, snapshot, nil), "gate": accountForTest("gate", 7, HoldingsSnapshot{}, func(context.Context) (HoldingsSnapshot, error) {
		return HoldingsSnapshot{}, errors.New("safe provider failure")
	})}
	root.others["gate"].snapshot = nil
	result, err := root.allHoldings(context.Background(), 7)
	if err != nil || result.Complete || len(result.Holdings) != 1 || len(result.Warnings) != 1 || result.Accounts[2].Status != "failed" {
		t.Fatalf("partial data lost: %+v %v", result, err)
	}
	root.others["binance"].snapshot = nil
	root.others["binance"].loader = func(context.Context) (HoldingsSnapshot, error) { return HoldingsSnapshot{}, errors.New("safe failure") }
	if _, err = root.allHoldings(context.Background(), 7); err == nil {
		t.Fatal("all-account failure presented as empty success")
	}
}

func TestPriceAlertQuotesEnforceOwnershipAndQuoteFreshness(t *testing.T) {
	snapshot := HoldingsSnapshot{AsOf: time.Now().UTC().Format(time.RFC3339), Complete: true, Holdings: []AccountHolding{{HoldingID: "spot:BTC", StockResponse: StockResponse{ID: "BTC", Currency: "USDT", CurrentPrice: 100}}}}
	root := &AccountHandler{cfg: config.BitgetConfig{APIKey: "key", APISecret: "secret", Passphrase: "pass", OwnerUserID: 7}, snapshot: &snapshot, cachedAt: time.Now()}
	if _, err := root.PriceAlertQuotes(context.Background(), 8, "bitget"); err == nil {
		t.Fatal("price monitor bypassed owner binding")
	}
	rows, err := root.PriceAlertQuotes(context.Background(), 7, "bitget")
	if err != nil || len(rows) != 1 || !rows[0].Fresh || rows[0].HoldingID != "bitget:spot:BTC" {
		t.Fatal("fresh holdings unavailable")
	}
	snapshot.Holdings[0].QuoteTime = "1700000000000"
	rows, err = root.PriceAlertQuotes(context.Background(), 7, "bitget")
	if err != nil || rows[0].Fresh {
		t.Fatal("old provider quote accepted")
	}
	snapshot.Holdings[0].QuoteTime = ""
	snapshot.Stale = true
	rows, err = root.PriceAlertQuotes(context.Background(), 7, "bitget")
	if err != nil || rows[0].Fresh {
		t.Fatal("stale snapshot accepted")
	}
}
