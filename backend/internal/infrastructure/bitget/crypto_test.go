package bitget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCryptoQuotesPreserveIdentityOrderCurrencyAndTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/market/instruments" {
			w.Write([]byte(`{"code":"00000","data":[{"symbol":"RUNEUSDT","baseCoin":"RUNE","quoteCoin":"USDT","symbolType":"crypto","status":"online"},{"symbol":"BTCUSDT","baseCoin":"BTC","quoteCoin":"USDT","symbolType":"crypto","status":"online"},{"symbol":"RAAPLUSDT","baseCoin":"rAAPL","quoteCoin":"USDT","symbolType":"stock","status":"online"}]}`))
			return
		}
		w.Write([]byte(`{"code":"00000","data":[{"symbol":"RUNEUSDT","lastPrice":"2","openPrice24h":"1","price24hPcnt":"1","ts":"1792565189000"},{"symbol":"BTCUSDT","lastPrice":"80000","openPrice24h":"70000","price24hPcnt":"0.142857","ts":"1792565189000"},{"symbol":"RAAPLUSDT","lastPrice":"200"}]}`))
	}))
	defer server.Close()
	c := NewClient(server.URL)
	quotes, err := c.CryptoQuotes(context.Background(), []string{"BTC/USDT", "RUNE", "RAAPL"})
	if err != nil || len(quotes) != 2 {
		t.Fatalf("quotes: %+v %v", quotes, err)
	}
	if quotes[0].Code != "BTC" || quotes[1].Code != "RUNE" || quotes[0].Currency != "USDT" || quotes[0].Market != "crypto" || quotes[0].Timestamp != "1792565189000" || quotes[1].Change != 1 {
		t.Fatalf("identity, units or time changed: %+v", quotes)
	}
	if _, err := c.CryptoCandles(context.Background(), "RAAPL", "1h", 24); err == nil {
		t.Fatal("stock instrument used for a crypto chart")
	}
}
