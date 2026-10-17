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
		fields[3] = "200"
		fields[4] = "190"
		fields[31] = "10"
		fields[32] = "5.26"
		body := `v_usMSFT="` + strings.Join(fields, "~") + `";`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	rows, err := h.fetchUSStocksBySymbol(context.Background(), "AAPL,MSFT")
	if err != nil || len(rows) != 2 {
		t.Fatalf("quotes %+v: %v", rows, err)
	}
	if rows[0].ID != "AAPL" || rows[0].Source != "bitget" || rows[1].ID != "MSFT" || rows[1].Source != "tencent" || rows[1].Currency != "USD" {
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
	s := NewFundamentalsService("", "")
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

func TestFMPForwardPENeverMixesEPSCurrencyOrConsensusWithGuidance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/income-statement-ttm":
			w.Write([]byte(`[{"epsDiluted":4,"reportedCurrency":"CNY"}]`))
		case "/ratios-ttm":
			w.Write([]byte(`[{"priceToEarningsRatioTTM":12}]`))
		case "/profile":
			w.Write([]byte(`[{"price":100,"currency":"HKD"}]`))
		case "/analyst-estimates":
			w.Write([]byte(`[{"date":"2099-12-31","epsAvg":5}]`))
		case "/earnings":
			w.Write([]byte(`[{"date":"2020-01-01"},{"date":"2099-10-14"}]`))
		}
	}))
	defer server.Close()
	s := NewFundamentalsService("test-key", "")
	s.BaseURL = server.URL
	row := s.Fetch(context.Background(), "hk_stock", "00700")
	if row.ForwardPE != nil || row.GuidanceEPS != nil || row.ForwardEPS == nil || *row.ForwardEPS != 5 || row.NextEarningsDate != "2099-10-14" {
		t.Fatalf("misleading fundamental values: %+v", row)
	}
}

func TestFMPSubscriptionErrorsAreExplicitAndSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/profile" {
			w.Write([]byte(`[{"price":100,"currency":"USD"}]`))
			return
		}
		if r.URL.Query().Get("symbol") == "PARTIAL" && r.URL.Path == "/ratios-ttm" {
			w.Write([]byte(`[{"priceToEarningsRatioTTM":12}]`))
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte("Restricted Endpoint: private-api-key https://example.com/?apikey=private-api-key"))
	}))
	defer server.Close()
	s := NewFundamentalsService("private-api-key", "")
	s.BaseURL = server.URL
	for _, symbol := range []string{"AVGO", "PARTIAL"} {
		row := s.Fetch(context.Background(), "us_stock", symbol)
		if !strings.Contains(row.Message, "EPS：当前 FMP 订阅无接口权限（HTTP 402）") || !strings.Contains(row.Message, "财报日期：") {
			t.Fatalf("missing subscription diagnosis: %+v", row)
		}
		if strings.Contains(row.Message, "private-api-key") || strings.Contains(row.Message, "https://") {
			t.Fatal("upstream credential leaked")
		}
		if row.EPS != nil || row.NextEarningsDate != "" || row.ForwardPE != nil {
			t.Fatal("fabricated restricted data")
		}
		if symbol == "PARTIAL" && (row.PE == nil || *row.PE != 12 || row.Status != "available") {
			t.Fatal("partial available data lost")
		}
		if symbol == "AVGO" && row.Status != "unavailable" {
			t.Fatal("restricted data marked available")
		}
	}
}
