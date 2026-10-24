package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
)

type quoteTransport func(*http.Request) (*http.Response, error)

func (f quoteTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUSQuotePartialFallbackOnlyRequestsMissingSymbols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/market/instruments":
			w.Write([]byte(`{"code":"00000","data":[{"symbol":"RAAPLUSDT","baseCoin":"rAAPL","quoteCoin":"USDT","symbolType":"stock","isReality":"yes","status":"online"}]}`))
		case "/api/v3/market/tickers":
			w.Write([]byte(`{"code":"00000","data":[{"symbol":"RAAPLUSDT","lastPrice":"110","openPrice24h":"100","price24hPcnt":"0.1"}]}`))
		default:
			w.Write([]byte(`{"code":"00000","data":[]}`))
		}
	}))
	defer server.Close()
	h := &StockHandler{bitget: bitget.NewClient(server.URL), httpClient: &http.Client{Transport: quoteTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/q=usMSFT" {
			t.Errorf("unexpected fallback query %s", r.URL.Path)
		}
		fields := make([]string, 50)
		fields[1] = "Microsoft"
		fields[2] = "MSFT.OQ"
		fields[3] = "200"
		fields[4] = "190"
		fields[31] = "10"
		fields[32] = "5.26"
		fields[30] = "2026-10-20 16:00:01"
		body := `v_usMSFT="` + strings.Join(fields, "~") + `";`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	rows, err := h.fetchUSStocksBySymbol(context.Background(), "AAPL,MSFT")
	if err != nil || len(rows) != 2 {
		t.Fatalf("quotes %+v: %v", rows, err)
	}
	if rows[0].ID != "AAPL" || rows[0].Source != "bitget" || rows[1].ID != "MSFT" || rows[1].Source != "tencent" || rows[1].ProviderSymbol != "MSFT.OQ" || rows[1].Currency != "USD" || rows[1].QuoteTime != "2026-10-20T16:00:01-04:00" || rows[1].QuoteSession != "close" {
		t.Fatalf("mixed-source quotes wrong: %+v", rows)
	}
}

func TestMergeWatchlistKeepsMissingQuotesAndDeduplicatesLegacyCodes(t *testing.T) {
	items := []model.WatchlistItem{{Market: "a_share", StockCode: "600519", Symbol: "SH600519", Name: "茅台"}, {Market: "a_share", StockCode: "sh600519", Name: "茅台"}, {Market: "hk_stock", StockCode: "700", Name: "腾讯"}, {Market: "crypto", StockCode: "btc_price", Name: "Bitcoin"}}
	quotes := []StockResponse{{ID: "sh600519", Market: "a_share", CurrentPrice: 10}, {ID: "BTC", Market: "crypto", CurrentPrice: 20}}
	rows := mergeWatchlistQuotes(items, quotes)
	if len(rows) != 3 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if rows[0].ID != "sh600519" || rows[0].Name != "茅台" || rows[1].ID != "00700" || rows[1].QuoteAvailable == nil || *rows[1].QuoteAvailable || rows[2].CurrentPrice != 20 {
		t.Fatalf("lost or misidentified records: %+v", rows)
	}
}

func TestFundamentalsDoNotInventDataOrAcceptUntraceableGuidance(t *testing.T) {
	s := NewFundamentalsService("")
	s.SEC = nil
	s.Yahoo, s.FinnhubAPIKey = nil, ""
	row := s.Fetch(context.Background(), "us_stock", "AAPL")
	data, _ := json.Marshal(row)
	if !strings.Contains(string(data), `"eps":null`) || row.Status != "unavailable" {
		t.Fatalf("missing data became real data: %s", data)
	}
	if crypto := s.Fetch(context.Background(), "crypto", "BTC"); crypto.Status != "not_applicable" {
		t.Fatal("crypto has company fundamentals")
	}
	file := filepath.Join(t.TempDir(), "fundamentals.json")
	os.WriteFile(file, []byte(`{"us_stock:AAPL":{"source":"company filing","as_of":"2026-10-14","guidance_eps":2.5}}`), 0600)
	s.File = file
	if row = s.Fetch(context.Background(), "us_stock", "AAPL"); row.GuidanceEPS != nil {
		t.Fatal("untraceable guidance accepted")
	}
}

func TestMarketCacheAllowsOnlyKnownPrimaryAndFallbackSources(t *testing.T) {
	for _, c := range []struct {
		market, source string
		allowed        bool
	}{{"us_stock", "bitget", true}, {"us_stock", "tencent", true}, {"hk_stock", "tencent", true}, {"crypto", "binance", true}, {"crypto", "tencent", false}, {"us_stock", "binance", false}, {"us_stock", "", false}, {"a_share", "", true}} {
		if got := bitgetBoardCacheAllowed(c.market, []IndexResponse{{Source: c.source}}); got != c.allowed {
			t.Fatalf("%s %s cache allowed=%v", c.market, c.source, got)
		}
	}
}

func TestHongKongFallbackPreservesExchangeTimeAndHKD(t *testing.T) {
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer failed.Close()
	h := &StockHandler{bitget: bitget.NewClient(failed.URL), httpClient: &http.Client{Transport: quoteTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/q=hk00700" {
			t.Fatalf("incorrect HK fallback query: %s", r.URL.Path)
		}
		fields := make([]string, 50)
		fields[1] = "Tencent"
		fields[3] = "400.4"
		fields[30] = "2026/10/21 14:31:24"
		body := `v_hk00700="` + strings.Join(fields, "~") + `";`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	rows, err := h.fetchHKStocksByCode(context.Background(), []string{"700"})
	if err != nil || len(rows) != 1 || rows[0].Currency != "HKD" || rows[0].Source != "tencent" || rows[0].QuoteTime != "2026-10-21T14:31:24+08:00" || rows[0].QuoteSession == "close" {
		t.Fatalf("HK fallback: %+v %v", rows, err)
	}
}

func TestTencentTimestampUsesExchangeTimezoneAndPreservesMissingTime(t *testing.T) {
	if got := tencentQuoteTime("2026-12-01 16:00:01", "us_stock"); got != "2026-12-01T16:00:01-05:00" {
		t.Fatal(got)
	}
	if got := tencentQuoteTime("missing", "us_stock"); got != "" {
		t.Fatalf("invented quote time %s", got)
	}
}

func TestCryptoFallbackRetainsBinanceQuoteTimestamp(t *testing.T) {
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer failed.Close()
	h := &StockHandler{bitget: bitget.NewClient(failed.URL), httpClient: &http.Client{Transport: quoteTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "data-api.binance.vision" {
			t.Fatalf("unexpected fallback host: %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"symbol":"BTCUSDT","lastPrice":"82000","closeTime":1792565189000}]`))}, nil
	})}}
	rows, err := h.fetchBitgetCryptoStocks(context.Background(), []string{"BTCUSDT"})
	if err != nil || len(rows) != 1 || rows[0].Source != "binance" || rows[0].Currency != "USDT" || rows[0].CurrentPrice != 82000 || rows[0].QuoteTime != "2026-10-21T06:46:29Z" {
		t.Fatalf("crypto fallback timestamp or currency changed: %+v %v", rows, err)
	}
}
