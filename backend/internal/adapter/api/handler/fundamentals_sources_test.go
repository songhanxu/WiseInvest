package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func yahooFixture(t *testing.T, serve http.HandlerFunc) *yahooFundamentalsClient {
	t.Helper()
	server := httptest.NewServer(serve)
	t.Cleanup(server.Close)
	y := newYahooFundamentalsClient()
	y.baseURL, y.cookieURL, y.crumbURL = server.URL, server.URL+"/cookie", server.URL+"/crumb"
	return y
}

func TestYahooPreservesZeroCurrencyAndDateRange(t *testing.T) {
	var cookies, crumbs, quotes atomic.Int32
	start := time.Now().AddDate(0, 0, 10).UTC().Truncate(24 * time.Hour).Add(21 * time.Hour)
	end := start.Add(48 * time.Hour)
	y := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			cookies.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "A3", Value: "session", Path: "/"})
			w.WriteHeader(http.StatusNotFound)
		case "/crumb":
			crumbs.Add(1)
			if _, err := r.Cookie("A3"); err != nil {
				t.Error("cookie not retained")
			}
			fmt.Fprint(w, "session-crumb")
		case "/v10/finance/quoteSummary/0700.HK":
			quotes.Add(1)
			if r.URL.Query().Get("crumb") != "session-crumb" {
				t.Error("missing crumb")
			}
			fmt.Fprintf(w, `{"quoteSummary":{"error":null,"result":[{"price":{"symbol":"0700.HK","quoteType":"EQUITY","currency":"HKD","exchangeTimezoneName":"Asia/Hong_Kong"},"financialData":{"financialCurrency":"CNY"},"defaultKeyStatistics":{"trailingEps":{"raw":0},"forwardEps":3,"forwardPE":15},"summaryDetail":{"trailingPE":18},"calendarEvents":{"earnings":{"earningsDate":[%d,{"raw":%d}]}},"assetProfile":{"longBusinessSummary":"Company."}}]}}`, start.Unix(), end.Unix())
		default:
			t.Errorf("unexpected Yahoo path %s", r.URL.Path)
		}
	})
	s := NewFundamentalsService("")
	s.SEC = nil
	s.Yahoo, s.FinnhubAPIKey = y, ""
	file := filepath.Join(t.TempDir(), "verified.json")
	raw, _ := json.Marshal(map[string]FundamentalsResponse{"hk_stock:00700": {Source: "verified", AsOf: "2026-01-01", Currency: "CNY", EPS: floatPtr(99)}})
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s.File = file
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row := s.Fetch(context.Background(), "hk_stock", "00700")
			if row.Status != "available" || row.EPS == nil || *row.EPS != 99 || row.Currency != "CNY" || row.Metrics["eps"].Source != "verified" || row.Metrics["pe"].Source != "Yahoo Finance（标的股票）" || row.GuidanceEPS != nil {
				t.Errorf("wrong merged fundamentals: %+v", row)
			}
			zone, _ := time.LoadLocation("Asia/Hong_Kong")
			if row.NextEarningsDate != start.In(zone).Format("2006-01-02") || row.NextEarningsDateEnd != end.In(zone).Format("2006-01-02") || row.EarningsDateStatus != "estimated" || row.ForwardPeriod != "" {
				t.Errorf("wrong period or calendar range: %+v", row)
			}
		}()
	}
	wg.Wait()
	if cookies.Load() != 1 || crumbs.Load() != 1 || quotes.Load() != 1 {
		t.Fatal("concurrent calls did not share session and cache")
	}
	// The verified override must never mutate the cached provider data/provenance.
	s.File = ""
	row := s.Fetch(context.Background(), "hk_stock", "00700")
	if row.EPS == nil || *row.EPS != 0 || row.Metrics["eps"].Source != "Yahoo Finance（标的股票）" {
		t.Fatalf("cache was mutated: %+v", row)
	}
}

func TestYahooDecodeRejectsPastDatesAndPreservesMissingOrNegativeEPS(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		currency, eps string
		present       bool
	}{{`"USD"`, "-2.5", true}, {`"USD"`, "null", false}, {`""`, "5", false}} {
		body := fmt.Sprintf(`{"quoteSummary":{"result":[{"financialData":{"financialCurrency":%s},"defaultKeyStatistics":{"trailingEps":%s},"summaryDetail":{"trailingPE":-10},"calendarEvents":{"earnings":{"earningsDate":[1000]}}}]}}`, test.currency, test.eps)
		row := decodeYahooFundamentals(strings.NewReader(body), now)
		if (row.EPS != nil) != test.present || row.PE != nil || row.NextEarningsDate != "" || row.GuidanceEPS != nil {
			t.Fatalf("fabricated/missing values: %+v", row)
		}
	}
	for _, body := range []string{`{"quoteSummary":{"error":{"description":"private token"},"result":null}}`, `{"quoteSummary":{"result":[]}}`, `<html>bad gateway</html>`} {
		row := decodeYahooFundamentals(strings.NewReader(body), now)
		if row.Status != "unavailable" || strings.Contains(row.Message, "private token") {
			t.Fatal("invalid response leaked or marked available")
		}
	}
}

func TestYahooAndFinnhubMergeFieldsAndKeepActualSources(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, 12).Format("2006-01-02")
	y := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			w.WriteHeader(404)
		case "/crumb":
			fmt.Fprint(w, "crumb")
		case "/calendar/earnings":
			fmt.Fprintf(w, `{"earningsCalendar":[{"symbol":"AAPL","date":%q}]}`, date)
		default:
			fmt.Fprint(w, `{"quoteSummary":{"result":[{"price":{"currency":"HKD","regularMarketPrice":100},"financialData":{"financialCurrency":"CNY"},"defaultKeyStatistics":{"trailingEps":5,"forwardEps":6},"summaryDetail":{"trailingPE":12}}]}}`)
		}
	})
	s := NewFundamentalsService("")
	s.SEC = nil
	s.Yahoo, s.FinnhubAPIKey, s.FinnhubBaseURL = y, "test-token", y.baseURL
	row := s.Fetch(context.Background(), "us_stock", "AAPL")
	if row.Status != "available" || row.EPS == nil || *row.EPS != 5 || row.PE == nil || *row.PE != 12 || row.Metrics["eps"].Source != "Yahoo Finance（标的股票）" || row.Metrics["eps"].Currency != "CNY" || row.NextEarningsDate != date || row.Metrics["next_earnings_date"].Source != "Finnhub（财报日历）" {
		t.Fatalf("source merge failed: %+v", row)
	}
	if row.ForwardPE != nil || row.ForwardEPS == nil || *row.ForwardEPS != 6 || row.GuidanceEPS != nil {
		t.Fatalf("mixed quote currency with EPS or consensus with guidance: %+v", row)
	}
}

func TestYahooFailureDoesNotSuppressFinnhubAndErrorsAreSanitized(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, 12).Format("2006-01-02")
	y := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/calendar/earnings" {
			if r.URL.Query().Get("symbol") == "AAPL" {
				fmt.Fprintf(w, `{"earningsCalendar":[{"symbol":"AAPL","date":%q}]}`, date)
			} else {
				w.WriteHeader(403)
				fmt.Fprint(w, "private-token upstream-url")
			}
			return
		}
		w.WriteHeader(429)
		fmt.Fprint(w, "private-token upstream-url")
	})
	s := NewFundamentalsService("")
	s.SEC = nil
	s.Yahoo, s.FinnhubAPIKey, s.FinnhubBaseURL = y, "private-token", y.baseURL
	row := s.Fetch(context.Background(), "us_stock", "AAPL")
	if row.Status != "available" || row.NextEarningsDate != date || row.EPS != nil || !strings.Contains(row.Message, "HTTP 429") {
		t.Fatalf("calendar lost when Yahoo failed: %+v", row)
	}
	row = s.Fetch(context.Background(), "us_stock", "MSFT")
	if row.Status != "unavailable" || row.EPS != nil || !strings.Contains(row.Message, "HTTP 403") || strings.Contains(row.Message, "private-token") || strings.Contains(row.Message, "upstream-url") {
		t.Fatalf("invalid or unsanitized failure: %+v", row)
	}
}

func TestYahooRefreshesRejectedCrumbOnceAndBacksOffAuthFailure(t *testing.T) {
	var crumbs, queries atomic.Int32
	y := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			w.WriteHeader(404)
		case "/crumb":
			fmt.Fprintf(w, "crumb-%d", crumbs.Add(1))
		default:
			if queries.Add(1) == 1 {
				w.WriteHeader(401)
				return
			}
			if r.URL.Query().Get("crumb") != "crumb-2" {
				t.Error("did not refresh rejected crumb")
			}
			fmt.Fprint(w, `{"quoteSummary":{"result":[{"summaryDetail":{"trailingPE":12}}]}}`)
		}
	})
	if row := y.fetch(context.Background(), "us_stock", "AAPL"); row.PE == nil || crumbs.Load() != 2 || queries.Load() != 2 {
		t.Fatalf("auth retry failed: %+v", row)
	}
	var rateLimited atomic.Int32
	limited := yahooFixture(t, func(w http.ResponseWriter, r *http.Request) {
		rateLimited.Add(1)
		w.WriteHeader(429)
		fmt.Fprint(w, "secret-crumb")
	})
	for _, code := range []string{"AAPL", "MSFT"} {
		row := limited.fetch(context.Background(), "us_stock", code)
		if row.Status != "unavailable" || strings.Contains(row.Message, "secret-crumb") {
			t.Fatal("rate limit not sanitized")
		}
	}
	if rateLimited.Load() != 1 {
		t.Fatal("auth failures did not back off across symbols")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	limited.gate <- struct{}{}
	if _, message := limited.credentials(ctx, ""); message == "" {
		t.Error("auth waiter ignored cancellation")
	}
	<-limited.gate
}

func TestFinnhubFallbackMatchesSymbolAndSkipsPastOrMalformedDates(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, 12).Format("2006-01-02")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Finnhub-Token") != "private-token" || r.URL.Query().Get("symbol") != "AAPL" || strings.Contains(r.URL.RawQuery, "private-token") {
			t.Error("wrong Finnhub auth or symbol")
		}
		fmt.Fprintf(w, `{"earningsCalendar":[{"symbol":"MSFT","date":"2099-01-01"},{"symbol":"AAPL","date":"2020-01-01"},{"symbol":"AAPL","date":"2099-01-01"},{"symbol":"AAPL","date":"invalid"},{"symbol":"AAPL","date":%q,"epsEstimate":100}]}`, date)
	}))
	defer server.Close()
	s := NewFundamentalsService("")
	s.SEC = nil
	s.Yahoo, s.FinnhubBaseURL, s.FinnhubAPIKey = nil, server.URL, "private-token"
	row := s.Fetch(context.Background(), "us_stock", "AAPL")
	if row.Status != "available" || row.NextEarningsDate != date || row.EarningsDateStatus != "estimated" || row.EPS != nil || row.GuidanceEPS != nil || row.Metrics["next_earnings_date"].Source != "Finnhub（财报日历）" {
		t.Fatalf("wrong Finnhub calendar: %+v", row)
	}
	s.Fetch(context.Background(), "hk_stock", "00700")
	s.Fetch(context.Background(), "us_stock", "SPY")
	if calls.Load() != 1 {
		t.Fatal("calendar used for unsupported market or ETF")
	}
}

func TestYahooSymbolMappingAndDynamicETF(t *testing.T) {
	for _, test := range []struct{ market, code, want string }{{"us_stock", "brk-b", "BRK-B"}, {"hk_stock", "00700", "0700.HK"}, {"hk_stock", "09988", "9988.HK"}, {"a_share", "sh600519", "600519.SS"}, {"a_share", "SZ000001", "000001.SZ"}, {"a_share", "bj920001", "920001.BJ"}, {"a_share", "600519.SS", "600519.SS"}} {
		if got := yahooStockSymbol(test.market, test.code); got != test.want {
			t.Errorf("%s: got %s want %s", test.code, got, test.want)
		}
	}
	row := decodeYahooFundamentals(strings.NewReader(`{"quoteSummary":{"result":[{"price":{"quoteType":"ETF"},"defaultKeyStatistics":{"trailingEps":5}}]}}`), time.Now())
	if row.Status != "not_applicable" || row.EPS != nil {
		t.Fatal("dynamic ETF has company EPS")
	}
}
