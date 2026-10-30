package bitget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicBitgetClientBacksOffAfterProviderFailure(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"code":"00000","data":[]}`))
	}))
	defer server.Close()
	c := NewClient(server.URL)
	var rows []Instrument
	if err := c.get(context.Background(), "/api/v3/market/instruments", nil, &rows); err == nil || c.backoff != 30*time.Second {
		t.Fatalf("initial failure did not back off: %v, %s", err, c.backoff)
	}
	if err := c.get(context.Background(), "/api/v3/market/instruments", nil, &rows); err == nil || calls.Load() != 1 {
		t.Fatalf("outage retried upstream too soon: %v, calls=%d", err, calls.Load())
	}
	c.failureMu.Lock()
	c.retryAt = time.Time{}
	c.failureMu.Unlock()
	if err := c.get(context.Background(), "/api/v3/market/instruments", nil, &rows); err == nil || c.backoff != time.Minute || calls.Load() != 2 {
		t.Fatalf("repeated failure did not extend backoff: %v, %s, calls=%d", err, c.backoff, calls.Load())
	}
	c.failureMu.Lock()
	c.retryAt = time.Time{}
	c.failureMu.Unlock()
	fail.Store(false)
	if err := c.get(context.Background(), "/api/v3/market/instruments", nil, &rows); err != nil || c.backoff != 0 || !c.retryAt.IsZero() || calls.Load() != 3 {
		t.Fatalf("successful probe did not reset backoff: %v, %s, calls=%d", err, c.backoff, calls.Load())
	}
}

func TestPublicBitgetClientBacksOffAfterConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := server.URL
	server.Close()
	c := NewClient(address)
	var rows []Instrument
	if err := c.get(context.Background(), "/api/v3/market/instruments", nil, &rows); err == nil || c.backoff != 30*time.Second {
		t.Fatalf("transport failure did not back off: %v, %s", err, c.backoff)
	}
}

func TestOptionalStockInfoFailureDoesNotBlockQuotes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/market/instruments":
			_, _ = w.Write([]byte(`{"code":"00000","data":[{"symbol":"RAAPLUSDT","baseCoin":"rAAPL","quoteCoin":"USDT","symbolType":"stock","isReality":"yes","status":"online"}]}`))
		case "/api/v3/reality/market/stock-info":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/api/v3/market/tickers":
			_, _ = w.Write([]byte(`{"code":"00000","data":[{"symbol":"RAAPLUSDT","lastPrice":"110","openPrice24h":"100"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	c := NewClient(server.URL)
	quotes, err := c.Quotes(context.Background(), "us_stock", []string{"AAPL"})
	if err != nil || len(quotes) != 1 || c.backoff != 0 {
		t.Fatalf("optional stock info failure suppressed primary quote: %+v %v backoff=%s", quotes, err, c.backoff)
	}
}

func TestStockMarketsKeepIdentityCurrencyAndChangeBasis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data interface{}
		switch r.URL.Path {
		case "/api/v3/market/instruments":
			if r.URL.Query().Get("category") == "SPOT" {
				data = []Instrument{{Symbol: "RAAPLUSDT", BaseCoin: "rAAPL", QuoteCoin: "USDT", SymbolType: "stock", IsReality: "yes", Status: "online"}, {Symbol: "BTCUSDT", BaseCoin: "BTC", SymbolType: "crypto", Status: "online"}}
			} else {
				data = []Instrument{{Symbol: "TENCENTHKDUSDT", BaseCoin: "TENCENTHKD", SymbolType: "stock", Status: "online"}, {Symbol: "TSLAUSDT", BaseCoin: "TSLA", SymbolType: "stock", Status: "online"}}
			}
		case "/api/v3/reality/market/stock-info":
			data = []map[string]string{{"symbol": "RAAPLUSDT", "code": "AAPL", "name": "Apple Inc."}}
		case "/api/v3/market/tickers":
			data = []Ticker{{Symbol: "RAAPLUSDT", Last: "110", Open: "100", Change: "0.1"}, {Symbol: "TENCENTHKDUSDT", Last: "420", Open: "400", Change: "0.05"}}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": "00000", "data": data})
	}))
	defer server.Close()
	client := NewClient(server.URL)
	us, err := client.Quotes(context.Background(), "us_stock", []string{"AAPL"})
	if err != nil || len(us) != 1 {
		t.Fatalf("quotes %v: %v", us, err)
	}
	if us[0].Code != "AAPL" || us[0].Currency != "USDT" || us[0].ChangePercent != 10 || us[0].Change != 10 {
		t.Fatalf("incorrect stock token units: %+v", us[0])
	}
	hk, err := client.Search(context.Background(), "hk_stock", "700")
	if err != nil || len(hk) != 1 {
		t.Fatalf("HK search %v: %v", hk, err)
	}
	if hk[0].Code != "00700" || hk[0].Currency != "HKD" || hk[0].ProductType != "quanto_perpetual" {
		t.Fatalf("incorrect HK identity: %+v", hk[0])
	}
	if us, _ := client.Search(context.Background(), "hk_stock", "TSLA"); len(us) != 0 {
		t.Fatal("US perpetual leaked into HK market")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			market, code := "us_stock", "AAPL"
			if i%2 == 0 {
				market, code = "hk_stock", "00700"
			}
			quotes, err := client.Quotes(context.Background(), market, []string{code})
			if err != nil || len(quotes) != 1 {
				t.Errorf("concurrent quote failed: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestAggregatePreservesOHLCAndMondayWeekBoundary(t *testing.T) {
	date := func(v string) time.Time { d, _ := time.Parse(time.RFC3339, v); return d }
	rows := []Candle{{Time: date("2026-10-11T00:00:00Z"), Open: 10, High: 15, Low: 8, Close: 12, Volume: 2}, {Time: date("2026-10-12T00:00:00Z"), Open: 12, High: 17, Low: 9, Close: 14, Volume: 3}, {Time: date("2026-10-13T00:00:00Z"), Open: 14, High: 19, Low: 11, Close: 16, Volume: 4}}
	weekly := Aggregate(rows, "1w")
	if len(weekly) != 2 {
		t.Fatalf("weekly bars: %+v", weekly)
	}
	bar := weekly[1]
	if bar.Time.Format("2006-01-02") != "2026-10-12" || bar.Open != 12 || bar.Close != 16 || bar.High != 19 || bar.Low != 9 || bar.Volume != 7 {
		t.Fatalf("invalid aggregate: %+v", bar)
	}
	rows[0].Time = date("2026-10-14T10:00:00Z")
	rows[1].Time = date("2026-10-14T10:15:00Z")
	rows[2].Time = date("2026-10-14T10:30:00Z")
	if halfHour := Aggregate(rows, "30m"); len(halfHour) != 2 || halfHour[0].Open != 10 || halfHour[0].Close != 14 || halfHour[0].Volume != 5 {
		t.Fatalf("invalid half-hour aggregate: %+v", halfHour)
	}
}

func TestCandlesSortNewestFirstUpstreamAndRejectProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/market/instruments":
			json.NewEncoder(w).Encode(map[string]interface{}{"code": "00000", "data": []Instrument{{Symbol: "RAAPLUSDT", BaseCoin: "rAAPL", QuoteCoin: "USDT", SymbolType: "stock", IsReality: "yes", Status: "online"}}})
		case "/api/v3/reality/market/stock-info":
			w.Write([]byte(`{"code":"00000","data":[]}`))
		default:
			if r.URL.Query().Get("interval") != "1D" {
				w.Write([]byte(`{"code":"40017","data":null}`))
				return
			}
			w.Write([]byte(`{"code":"00000","data":[["1791936000000","12","15","11","14","3","0"],["1791849600000","10","13","9","12","2","0"]]}`))
		}
	}))
	defer server.Close()
	c := NewClient(server.URL)
	rows, err := c.Candles(context.Background(), "us_stock", "AAPL", "1d", 2)
	if err != nil || len(rows) != 2 || !rows[0].Time.Before(rows[1].Time) {
		t.Fatalf("candles %+v: %v", rows, err)
	}
	if _, err = c.Candles(context.Background(), "us_stock", "AAPL", "5m", 2); err == nil {
		t.Fatal("provider error silently accepted")
	}
}
