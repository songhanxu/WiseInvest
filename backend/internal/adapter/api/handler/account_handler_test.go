package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

type accountFixture struct {
	assets                []bitget.Asset
	positions             []bitget.Position
	assetErr, positionErr error
	calls                 atomic.Int32
}

func (f *accountFixture) Assets(context.Context, string) ([]bitget.Asset, error) {
	f.calls.Add(1)
	return f.assets, f.assetErr
}
func (f *accountFixture) Positions(context.Context, string) ([]bitget.Position, error) {
	f.calls.Add(1)
	return f.positions, f.positionErr
}

func TestAccountOwnershipBeforeNetworkAndCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := &accountFixture{}
	h := &AccountHandler{cfg: config.BitgetConfig{APIKey: "key", APISecret: "secret", Passphrase: "pass", OwnerUserID: 7, AccountName: "Private name", AccountType: "uta"}, private: f, snapshot: &HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{Quantity: "secret quantity"}}}, cachedAt: time.Now()}
	request := func(user uint) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("userID", user)
		c.Params = gin.Params{{Key: "accountID", Value: "bitget"}}
		c.Request = httptest.NewRequest("GET", "/accounts/bitget/holdings", nil)
		h.GetHoldings(c)
		return w
	}
	if w := request(8); w.Code != 403 || strings.Contains(w.Body.String(), "secret quantity") || f.calls.Load() != 0 {
		t.Fatalf("owner isolation failed: %s", w.Body)
	}
	if info := h.summary(8); info.Name != "Bitget" || info.AccountType != "" {
		t.Fatal("owner metadata exposed")
	}
	if w := request(7); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("owner cache unavailable: %s", w.Body)
	}
	h.cfg.OwnerUserID = 0
	if w := request(7); w.Code != 503 || f.calls.Load() != 0 {
		t.Fatal("zero owner granted access")
	}
	h.cfg.OwnerUserID = 7
	h.cfg.APISecret = ""
	if w := request(7); w.Code != 503 || f.calls.Load() != 0 {
		t.Fatal("incomplete credentials granted access")
	}
}

func accountMarketFixture(t *testing.T, failQuotes bool) *bitget.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch r.URL.Path {
		case "/api/v3/market/instruments":
			if r.URL.Query().Get("category") == "SPOT" {
				data = []bitget.Instrument{{Symbol: "RAAPLUSDT", BaseCoin: "rAAPL", QuoteCoin: "USDT", IsReality: "yes", SymbolType: "stock"}, {Symbol: "RANDOMUSDT", BaseCoin: "RANDOM", SymbolType: "crypto"}}
			} else {
				data = []bitget.Instrument{{Symbol: "TENCENTHKDUSDT", BaseCoin: "TENCENTHKD", QuoteCoin: "USDT", SymbolType: "stock"}, {Symbol: "BTCUSDT", BaseCoin: "BTC", SymbolType: "crypto"}}
			}
		case "/api/v3/market/tickers":
			if failQuotes {
				w.WriteHeader(503)
				return
			}
			data = []bitget.Ticker{{Symbol: "RAAPLUSDT", Last: "210", Open: "200", Change: "0.05"}, {Symbol: "TENCENTHKDUSDT", Last: "420", Open: "400", Change: "0.05"}}
		}
		json.NewEncoder(w).Encode(map[string]any{"code": "00000", "data": data})
	}))
	t.Cleanup(server.Close)
	return bitget.NewClient(server.URL)
}

func TestAccountKeepsStocksCryptoUnknownAssetsAndHedgedPositions(t *testing.T) {
	f := &accountFixture{assets: []bitget.Asset{{Coin: "rAAPL", Balance: "0.100000001"}, {Coin: "RANDOM", Balance: "10"}, {Coin: "BTC", Balance: "1"}}, positions: []bitget.Position{{Symbol: "TENCENTHKDUSDT", Total: "2", Side: "long", MarginMode: "crossed"}, {Symbol: "TENCENTHKDUSDT", Total: "1", Side: "short", MarginMode: "crossed"}, {Symbol: "BTCUSDT", Total: "10", Side: "long"}}}
	h := &AccountHandler{cfg: config.BitgetConfig{AccountType: "uta", IncludeFutures: true}, private: f, market: accountMarketFixture(t, false)}
	data, err := h.readHoldings(context.Background())
	if err != nil || !data.Complete || len(data.Holdings) != 6 {
		t.Fatalf("holdings %+v: %v", data, err)
	}
	ids := make(map[string]bool)
	for _, row := range data.Holdings {
		if ids[row.HoldingID] {
			t.Fatal("hedged positions merged")
		}
		ids[row.HoldingID] = true
		if row.Market == "crypto" || row.Market == "other" {
			if row.ID != "BTC" && row.ID != "RANDOM" {
				t.Fatal("unexpected crypto identity")
			}
			continue
		}
		if row.HoldingType == "spot" {
			if row.ID != "AAPL" || row.Quantity != "0.100000001" || row.Currency != "USDT" || row.CurrentPrice != 210 {
				t.Fatalf("invalid spot: %+v", row)
			}
		} else if row.ID != "00700" || row.Market != "hk_stock" || row.Currency != "HKD" || row.CurrentPrice != 420 {
			t.Fatalf("invalid HK position: %+v", row)
		}
	}
}

func TestAccountPartialFailureAndUnavailableQuotesRetainHoldings(t *testing.T) {
	f := &accountFixture{assets: []bitget.Asset{{Coin: "rAAPL", Balance: "2"}}, positionErr: errors.New("sensitive upstream response")}
	h := &AccountHandler{cfg: config.BitgetConfig{AccountType: "uta", IncludeFutures: true}, private: f, market: accountMarketFixture(t, true)}
	data, err := h.readHoldings(context.Background())
	if err != nil || data.Complete || len(data.Holdings) != 1 || len(data.Warnings) != 2 {
		t.Fatalf("partial holdings %+v: %v", data, err)
	}
	if row := data.Holdings[0]; row.Quantity != "2" || row.QuoteAvailable == nil || *row.QuoteAvailable {
		t.Fatal("position hidden or quote fabricated")
	}
	f.assetErr = errors.New("sensitive credentials")
	if _, err := h.readHoldings(context.Background()); err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("failed account read accepted or unsafe error exposed")
	}
}

func TestEmptyAccountDoesNotRequirePublicMarket(t *testing.T) {
	h := &AccountHandler{cfg: config.BitgetConfig{AccountType: "uta", IncludeFutures: true}, private: &accountFixture{}}
	data, err := h.readHoldings(context.Background())
	if err != nil || !data.Complete || data.Holdings == nil || len(data.Holdings) != 0 {
		t.Fatalf("empty account %+v: %v", data, err)
	}
}

func TestHoldingsExposeSeparateSafeProviderErrors(t *testing.T) {
	f := &accountFixture{assetErr: &bitget.AccountError{Code: "40038", HTTPStatus: 400}, positionErr: &bitget.AccountError{Code: "40014", HTTPStatus: 400}}
	h := &AccountHandler{cfg: config.BitgetConfig{AccountType: "uta", IncludeFutures: true}, private: f}
	_, err := h.readHoldings(context.Background())
	if err == nil {
		t.Fatal("failure accepted")
	}
	for _, want := range []string{"资产查询", "40038", "合约查询", "40014", "uta"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing diagnostic %s: %v", want, err)
		}
	}
}

func TestUSDTBalanceNeverUsesFiatTradingPair(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		if r.URL.Path == "/api/v3/market/instruments" {
			data = []bitget.Instrument{{Symbol: "USDT_VND", BaseCoin: "USDT", QuoteCoin: "VND", SymbolType: "crypto"}}
		} else {
			data = []bitget.Ticker{{Symbol: "USDT_VND", Last: "25951"}}
		}
		json.NewEncoder(w).Encode(map[string]any{"code": "00000", "data": data})
	}))
	defer server.Close()
	h := &AccountHandler{cfg: config.BitgetConfig{AccountType: "uta"}, private: &accountFixture{assets: []bitget.Asset{{Coin: "USDT", Balance: "1454.69"}}}, market: bitget.NewClient(server.URL)}
	data, err := h.readHoldings(context.Background())
	if err != nil || len(data.Holdings) != 1 {
		t.Fatalf("holdings: %+v %v", data, err)
	}
	row := data.Holdings[0]
	if row.CurrentPrice != 1 || row.Currency != "USDT" || row.MarketValue != "1454.69" || row.ProviderSymbol != "" {
		t.Fatalf("USDT identity price overwritten by fiat quote: %+v", row)
	}
}
