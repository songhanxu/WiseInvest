package exchange

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureClient(provider string, fn func(*http.Request) (int, string)) *Client {
	c := NewClient(config.ExchangeAccountConfig{Provider: provider, APIKey: "key", APISecret: "secret", AccountType: "spot"})
	c.Now = func() time.Time { return time.UnixMilli(1700000000000) }
	c.HTTPClient.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		status, body := fn(r)
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	return c
}
func TestSignedReadOnlyHostsAndNoSecretsOnPublicRequests(t *testing.T) {
	for _, provider := range []string{"binance", "gate"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			c := fixtureClient(provider, func(r *http.Request) (int, string) {
				calls++
				if r.Method != "GET" {
					t.Fatal("mutation request")
				}
				if provider == "binance" {
					if r.URL.Host != "api.binance.com" {
						t.Fatal("unexpected signing host")
					}
					if r.URL.Path == "/api/v3/account" {
						if r.Header.Get("X-MBX-APIKEY") != "key" || r.URL.Query().Get("signature") != "f98849c4b4d023c32a8377514e4918140ac3f2cfa817c944db3400bef0f7fa0a" {
							t.Fatal("invalid Binance signature")
						}
					} else if r.Header.Get("X-MBX-APIKEY") != "" || r.URL.Query().Has("signature") {
						t.Fatal("private credentials attached to public quote")
					}
				} else {
					if r.URL.Host != "api.gateio.ws" {
						t.Fatal("unexpected signing host")
					}
					if r.URL.Path == "/api/v4/spot/accounts" {
						if r.Header.Get("KEY") != "key" || r.Header.Get("Timestamp") != "1700000000" || r.Header.Get("SIGN") != "fa17722c057aa9b6bccc9bb5f91a1efe40a0ff7fa4af2ba9252028a29fab7bcb90a309b024e982d89e8bbf0997267c56498b0f4b69f306a79e80434b074f296e" {
							t.Fatal("invalid Gate signature")
						}
					} else if r.Header.Get("KEY") != "" || r.Header.Get("SIGN") != "" {
						t.Fatal("private credentials attached to public quote")
					}
				}
				return 200, `[]`
			})
			private, public := "/api/v3/account", "/api/v3/ticker/price"
			if provider == "gate" {
				private, public = "/spot/accounts", "/spot/tickers"
			}
			var data any
			if err := c.get(context.Background(), private, nil, &data); err != nil {
				t.Fatal(err)
			}
			if err := c.get(context.Background(), public, nil, &data); err != nil {
				t.Fatal(err)
			}
			if err := c.get(context.Background(), "/trade/place-order", nil, &data); err == nil || calls != 2 {
				t.Fatal("non-read path was requested")
			}
		})
	}
}
func TestRedirectRejectionAndProviderErrorsDoNotLeakSecrets(t *testing.T) {
	for _, status := range []int{301, 400, 401, 403, 429, 500} {
		calls := 0
		c := fixtureClient("binance", func(r *http.Request) (int, string) {
			calls++
			return status, `{"msg":"key secret account-id https://example.com/?signature=private"}`
		})
		// A custom transport with a redirect header exercises the redirect policy.
		if status == 301 {
			c.HTTPClient.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 301, Header: http.Header{"Location": {"https://example.com/steal"}}, Body: io.NopCloser(strings.NewReader("secret")), Request: r}, nil
			})
		}
		var data any
		err := c.get(context.Background(), "/api/v3/account", nil, &data)
		if err == nil || calls != 1 {
			t.Fatalf("error or redirect accepted: %v", err)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "https://") || strings.Contains(err.Error(), "account-id") {
			t.Fatal("raw provider response leaked")
		}
	}
}
func TestBinanceBalancesTokenMetadataAndHedgedFutures(t *testing.T) {
	c := fixtureClient("binance", func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/api/v3/account":
			return 200, `{"balances":[{"asset":"BTC","free":"0.1","locked":"0.2"},{"asset":"AAPLB","free":"2","locked":"0"},{"asset":"USDT","free":"1","locked":"0"},{"asset":"ZERO","free":"0","locked":"0"}]}`
		case "/api/v3/exchangeInfo":
			return 200, `{"symbols":[{"symbol":"BTCUSDT","baseAsset":"BTC","quoteAsset":"USDT"},{"symbol":"AAPLBUSDT","baseAsset":"AAPLB","quoteAsset":"USDT"}]}`
		case "/api/v3/ticker/price":
			return 200, `[{"symbol":"BTCUSDT","price":"100"},{"symbol":"AAPLBUSDT","price":"200"}]`
		case "/sapi/v1/equity/market/tokenized-assets":
			return 200, `[{"assetCode":"AAPLB","assetName":"Apple token","underlyingEquitySymbol":"AAPL"}]`
		case "/fapi/v3/positionRisk":
			return 200, `[{"symbol":"BTCUSDT","positionSide":"LONG","positionAmt":"0.2","entryPrice":"99","markPrice":"100","unRealizedProfit":"0.2","marginAsset":"USDT"},{"symbol":"BTCUSDT","positionSide":"SHORT","positionAmt":"-0.3","markPrice":"100","marginAsset":"USDT"}]`
		case "/fapi/v1/exchangeInfo":
			return 200, `{"symbols":[{"symbol":"BTCUSDT","baseAsset":"BTC","quoteAsset":"USDT","underlyingType":"COIN"}]}`
		}
		t.Errorf("unexpected endpoint %s", r.URL.Path)
		return 500, `{}`
	})
	c.cfg.IncludeStocks = true
	c.cfg.IncludeFutures = true
	result, err := c.Read(context.Background())
	if err != nil || len(result.Warnings) != 0 || len(result.Holdings) != 5 {
		t.Fatalf("unexpected result %+v %v", result, err)
	}
	if h := result.Holdings[0]; h.Quantity != "0.3" || num(h.MarketValue) != 30 || h.Quote.ProviderSymbol != "BTCUSDT" {
		t.Fatalf("wrong spot units: %+v", h)
	}
	if h := result.Holdings[1]; h.Quote.Code != "AAPL" || h.Unit != "AAPLB" || h.Quote.Market != "us_stock" || num(h.MarketValue) != 400 {
		t.Fatalf("token identity lost: %+v", h)
	}
	if h := result.Holdings[4]; h.Quantity != "0.3" || h.Side != "short" || h.ID == result.Holdings[3].ID {
		t.Fatalf("hedged positions merged: %+v", h)
	}
}
func TestGateStockAndContractCountsRemainSeparate(t *testing.T) {
	c := fixtureClient("gate", func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/api/v4/spot/accounts":
			return 200, `[{"currency":"BTC","available":"0.1","locked":"0.2"}]`
		case "/api/v4/spot/currency_pairs":
			return 200, `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","base_name":"Bitcoin"}]`
		case "/api/v4/spot/tickers":
			return 200, `[{"currency_pair":"BTC_USDT","last":"100"}]`
		case "/api/v4/stock/positions":
			return 200, `{"data":{"list":[{"symbol":"AAPL","exchange":"us","symbol_desc":"Apple","quote_currency":"USD","volume":"2","last_price":"200","avg_cost_price":"190","position_pnl":"20"}]}}`
		case "/api/v4/futures/usdt/positions":
			if r.URL.Query().Get("holding") != "true" {
				t.Error("missing holding filter")
			}
			return 200, `[{"contract":"BTC_USDT","size":-50,"mode":"dual_short","entry_price":"99","mark_price":"100","unrealised_pnl":"-0.5"}]`
		}
		return 500, `{}`
	})
	c.cfg.IncludeStocks = true
	c.cfg.IncludeFutures = true
	result, err := c.Read(context.Background())
	if err != nil || len(result.Holdings) != 3 || len(result.Warnings) != 0 {
		t.Fatalf("result %+v %v", result, err)
	}
	if h := result.Holdings[1]; h.Quote.Market != "us_stock" || h.Unit != "股" || h.Quantity != "2" || h.PnL != "20" || h.Quote.Currency != "USD" {
		t.Fatalf("invalid stock: %+v", h)
	}
	if h := result.Holdings[2]; h.Unit != "张" || h.Quantity != "50" || h.Side != "short" || h.MarketValue != "" || h.Quote.Market != "crypto" {
		t.Fatalf("contract count treated as base units: %+v", h)
	}
}
func TestPartialFailureAndUnifiedEquityDoNotBecomeEmpty(t *testing.T) {
	c := fixtureClient("gate", func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/api/v4/unified/accounts":
			return 200, `{"balances":{"USDT":{"equity":"-10","available":"999","freeze":"999"}}}`
		case "/api/v4/spot/currency_pairs", "/api/v4/spot/tickers":
			return 200, `[]`
		default:
			return 403, `{"message":"secret"}`
		}
	})
	c.cfg.AccountType = "unified"
	c.cfg.IncludeFutures = true
	result, err := c.Read(context.Background())
	if err != nil || len(result.Holdings) != 1 || len(result.Warnings) != 1 || result.Holdings[0].Quantity != "-10" || result.Holdings[0].Type != "balance" {
		t.Fatalf("lost partial equity: %+v %v", result, err)
	}
	c = fixtureClient("binance", func(*http.Request) (int, string) { return 200, `{}` })
	if _, err := c.Read(context.Background()); err == nil {
		t.Fatal("missing balances accepted as an empty account")
	}
	c = fixtureClient("gate", func(*http.Request) (int, string) { return 200, `null` })
	if _, err := c.Read(context.Background()); err == nil {
		t.Fatal("null balance accepted")
	}
}
func TestGateQuerySignaturePreservesQuery(t *testing.T) {
	c := fixtureClient("gate", func(r *http.Request) (int, string) {
		if r.URL.RawQuery != "holding=true" || r.Header.Get("SIGN") == "" {
			t.Fatal("unsigned position filter")
		}
		return 200, `[]`
	})
	var data any
	if err := c.get(context.Background(), "/futures/usdt/positions", url.Values{"holding": {"true"}}, &data); err != nil {
		t.Fatal(err)
	}
}
