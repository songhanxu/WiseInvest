package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

type blockingAccount struct {
	started, release chan struct{}
	calls            atomic.Int32
}

func (b *blockingAccount) Assets(ctx context.Context, _ string) ([]bitget.Asset, error) {
	b.calls.Add(1)
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-b.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (b *blockingAccount) Positions(context.Context, string) ([]bitget.Position, error) {
	return nil, nil
}

func TestHoldingsShareFlightAndAllowCancellation(t *testing.T) {
	reader := &blockingAccount{started: make(chan struct{}, 1), release: make(chan struct{})}
	h := &AccountHandler{private: reader}
	first := make(chan error, 1)
	go func() { _, err := h.holdings(context.Background()); first <- err }()
	<-reader.started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.holdings(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter ignored cancellation: %v", err)
	}
	close(reader.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if _, err := h.holdings(context.Background()); err != nil || reader.calls.Load() != 1 {
		t.Fatalf("did not share/cache flight: %v calls=%d", err, reader.calls.Load())
	}
}

func TestHoldingsFailureRetainsCompleteSnapshotAndBacksOff(t *testing.T) {
	reader := &accountFixture{assetErr: errors.New("offline")}
	h := &AccountHandler{cfg: config.BitgetConfig{AccountType: "uta"}, private: reader, snapshot: &HoldingsSnapshot{AccountID: "bitget", AsOf: "2026-01-01T00:00:00Z", LastSuccess: "2026-01-01T00:00:00Z", Complete: true, Holdings: []AccountHolding{{Quantity: "2"}}}, cachedAt: time.Now().Add(-time.Minute)}
	for i := 0; i < 3; i++ {
		data, err := h.holdings(context.Background())
		if err != nil || !data.Stale || data.Complete || len(data.Holdings) != 1 || data.LastSuccess != "2026-01-01T00:00:00Z" {
			t.Fatalf("lost stale snapshot: %+v %v", data, err)
		}
	}
	if reader.calls.Load() != 1 {
		t.Fatal("failure backoff did not prevent repeated requests")
	}
}

func TestFundamentalFlightCacheAndCurrencyProvenance(t *testing.T) {
	var calls atomic.Int32
	y := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			w.WriteHeader(404)
		case "/crumb":
			w.Write([]byte("crumb"))
		default:
			calls.Add(1)
			w.Write([]byte(`{"quoteSummary":{"result":[{"financialData":{"financialCurrency":"USD"},"defaultKeyStatistics":{"trailingEps":5},"assetProfile":{"longBusinessSummary":"Company profile."}}]}}`))
		}
	})
	file := filepath.Join(t.TempDir(), "verified.json")
	data := map[string]FundamentalsResponse{"us_stock:AAPL": {EPS: floatPtr(99), Currency: "CNY", Source: "verified", AsOf: time.Now().UTC().Format(time.RFC3339), GuidanceEPS: floatPtr(2), GuidancePeriod: "2026Q4", GuidanceBasis: "GAAP", GuidanceSourceURL: "https://example.com/filing", GuidanceReportDate: time.Now().AddDate(-1, 0, 0).Format("2006-01-02")}}
	raw, _ := json.Marshal(data)
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s := NewFundamentalsService(file)
	s.SEC = nil
	s.Yahoo, s.FinnhubAPIKey = y, ""
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row := s.Fetch(context.Background(), "us_stock", "AAPL")
			if row.EPS == nil || *row.EPS != 5 || row.Currency != "USD" || row.Metrics["eps"].Currency != "USD" || row.GuidanceEPS != nil || row.GuidancePeriod != "" || row.Description != "Company profile." {
				t.Errorf("mixed data: %+v", row)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("expected one Yahoo request for one flight, got %d", calls.Load())
	}
	for i := 0; i < 512; i++ {
		s.cache[time.Unix(int64(i), 0).String()] = fundamentalsCache{Time: time.Now()}
	}
	s.Fetch(context.Background(), "us_stock", "MSFT")
	if len(s.cache) > 512 {
		t.Fatalf("cache grew without eviction: %d", len(s.cache))
	}
}
func floatPtr(v float64) *float64 { return &v }

func TestProductDetailUsesContractAndRejectsUnknownProduct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category") != "USDT-FUTURES" {
			t.Errorf("wrong product category: %s", r.URL)
		}
		switch r.URL.Path {
		case "/api/v3/market/instruments":
			w.Write([]byte(`{"code":"00000","data":[{"symbol":"AAPLUSDT","baseCoin":"AAPL","quoteCoin":"USDT","symbolType":"stock"}]}`))
		case "/api/v3/market/tickers":
			w.Write([]byte(`{"code":"00000","data":[{"symbol":"AAPLUSDT","lastPrice":"321"}]}`))
		case "/api/v3/market/candles":
			w.Write([]byte(`{"code":"00000","data":[["1760000000000","320","322","319","321","2"]]}`))
		default:
			t.Errorf("unexpected upstream %s", r.URL.Path)
		}
	}))
	defer server.Close()
	h := &StockHandler{bitget: bitget.NewClient(server.URL)}
	router := gin.New()
	router.GET("/quote", h.GetStockQuote)
	router.GET("/kline", h.GetKLineData)
	for _, path := range []string{"/quote?code=AAPL&market=us_stock&category=USDT-FUTURES&provider_symbol=AAPLUSDT", "/kline?code=AAPL&market=us_stock&category=USDT-FUTURES&provider_symbol=AAPLUSDT&period=1h&limit=1"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %s", path, w.Body)
		}
		if strings.HasPrefix(path, "/quote") {
			var quote StockResponse
			if json.Unmarshal(w.Body.Bytes(), &quote) != nil || quote.ProviderSymbol != "AAPLUSDT" || quote.ProductType != "stock_perpetual" || quote.CurrentPrice != 321 {
				t.Fatalf("wrong product quote: %s", w.Body)
			}
		} else {
			var rows []KLineResponse
			if json.Unmarshal(w.Body.Bytes(), &rows) != nil || len(rows) != 1 || !strings.HasSuffix(rows[0].Date, "Z") {
				t.Fatalf("invalid product candles: %s", w.Body)
			}
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/quote?category=USDT-FUTURES&provider_symbol=OTHER", nil))
	if w.Code != 502 {
		t.Fatal("unknown instrument accepted")
	}
}

func TestCanonicalCryptoAliases(t *testing.T) {
	for _, code := range []string{"BTC", "btc_price", "BTCUSDT", "BTC/USDT", " btc "} {
		if got := canonicalWatchCode("crypto", code, ""); got != "BTC" {
			t.Errorf("%q normalized to %q", code, got)
		}
	}
}

func TestProductAnalysisCacheIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	code := "OPT_CACHE_TEST"
	setAnalysisToMemCache(code, "us_stock", "comprehensive", AnalysisResponse{Summary: "equity", Detail: "equity"})
	setAnalysisToMemCache(code+":USDT-FUTURES:AAPLUSDT", "us_stock", "comprehensive", AnalysisResponse{Summary: "perpetual", Detail: "perpetual"})
	h := &StockHandler{}
	router := gin.New()
	router.POST("/analysis", h.EnhanceAnalysis)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/analysis", strings.NewReader(`{"code":"OPT_CACHE_TEST","market":"us_stock","name":"Apple","category":"USDT-FUTURES","provider_symbol":"AAPLUSDT"}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "perpetual") || strings.Contains(w.Body.String(), "equity") {
		t.Fatalf("analysis mixed products: %s", w.Body)
	}
}
