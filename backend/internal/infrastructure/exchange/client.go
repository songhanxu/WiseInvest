// Package exchange reads account data only. It has no trading or transfer methods.
package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

type Holding struct {
	Quote                                                                                    bitget.Quote
	ID, Type, Side, Quantity, Unit, EntryPrice, PnL, MarginCurrency, MarginMode, MarketValue string
}
type Result struct {
	Holdings []Holding
	Warnings []string
}
type Client struct {
	cfg        config.ExchangeAccountConfig
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewClient(c config.ExchangeAccountConfig) *Client {
	return &Client{cfg: c, Now: time.Now, HTTPClient: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Hosts and paths are fixed, including authenticated metadata calls. Test
// transports intercept requests without permitting credential-host overrides.
func (c *Client) get(ctx context.Context, path string, params url.Values, target any) error {
	host := ""
	signed := false
	keyHeader := false
	if c.cfg.Provider == "binance" {
		switch path {
		case "/api/v3/account":
			host = "https://api.binance.com"
			signed = true
			keyHeader = true
		case "/fapi/v3/positionRisk":
			host = "https://fapi.binance.com"
			signed = true
			keyHeader = true
		case "/api/v3/ticker/price", "/api/v3/exchangeInfo":
			host = "https://api.binance.com"
		case "/fapi/v1/exchangeInfo":
			host = "https://fapi.binance.com"
		case "/sapi/v1/equity/market/tokenized-assets":
			host = "https://api.binance.com"
			keyHeader = true
		}
	} else if c.cfg.Provider == "gate" {
		switch path {
		case "/spot/accounts", "/unified/accounts", "/futures/usdt/positions", "/stock/positions":
			signed = true
			host = "https://api.gateio.ws"
		case "/spot/tickers", "/spot/currency_pairs", "/futures/usdt/contracts":
			host = "https://api.gateio.ws"
		}
	}
	if host == "" {
		return fmt.Errorf("unsupported read-only endpoint")
	}
	if (signed || keyHeader) && !c.cfg.HasCredentials() {
		return fmt.Errorf("API 凭证未配置")
	}
	q := url.Values{}
	for k, values := range params {
		q[k] = append([]string{}, values...)
	}
	headers := http.Header{"Accept": {"application/json"}, "Content-Type": {"application/json"}}
	if c.cfg.Provider == "binance" {
		if keyHeader {
			headers.Set("X-MBX-APIKEY", c.cfg.APIKey)
		}
		if signed {
			q.Set("timestamp", strconv.FormatInt(c.Now().UnixMilli(), 10))
			q.Set("recvWindow", "5000")
			mac := hmac.New(sha256.New, []byte(c.cfg.APISecret))
			mac.Write([]byte(q.Encode()))
			q.Set("signature", hex.EncodeToString(mac.Sum(nil)))
		}
	} else {
		path = "/api/v4" + path
		if signed {
			ts := strconv.FormatInt(c.Now().Unix(), 10)
			empty := sha512.Sum512(nil)
			pre := "GET\n" + path + "\n" + q.Encode() + "\n" + hex.EncodeToString(empty[:]) + "\n" + ts
			mac := hmac.New(sha512.New, []byte(c.cfg.APISecret))
			mac.Write([]byte(pre))
			headers.Set("KEY", c.cfg.APIKey)
			headers.Set("Timestamp", ts)
			headers.Set("SIGN", hex.EncodeToString(mac.Sum(nil)))
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("请求配置无效")
	}
	req.Header = headers
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("网络连接失败或查询超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("接口返回 HTTP %d，请检查账户类型、读取权限、IP 白名单及地区可用性", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("响应读取失败")
	}
	var envelope struct {
		Code  json.RawMessage `json:"code"`
		Label string          `json:"label"`
	}
	_ = json.Unmarshal(raw, &envelope)
	if envelope.Label != "" || (len(envelope.Code) > 0 && string(envelope.Code) != "0" && string(envelope.Code) != "200") {
		return fmt.Errorf("平台拒绝请求，请检查 API 权限与账户状态")
	}
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, target) != nil {
		return fmt.Errorf("平台返回格式异常")
	}
	return nil
}
func (c *Client) Read(ctx context.Context) (Result, error) {
	if c.cfg.Provider == "binance" {
		return c.binance(ctx)
	}
	return c.gate(ctx)
}
func num(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
func decimalSum(a, b string) string {
	r := new(big.Rat)
	for _, s := range []string{a, b} {
		if v, ok := new(big.Rat).SetString(s); ok {
			r.Add(r, v)
		}
	}
	s := r.FloatString(18)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}
func nonzero(s string) bool { v, ok := new(big.Rat).SetString(s); return ok && v.Sign() != 0 }
func abs(s string) string   { return strings.TrimPrefix(s, "-") }
func baseHolding(asset, quantity string) Holding {
	return Holding{Quote: bitget.Quote{Code: asset, Name: asset, Market: "crypto", Currency: "USDT", ProductType: "crypto_spot"}, ID: "spot:" + asset, Type: "spot", Side: "long", Quantity: quantity, Unit: asset}
}
func finishSpot(h *Holding, price string) {
	h.Quote.Price = num(price)
	if h.Quote.Price > 0 {
		h.MarketValue = bitget.QuantityValue(h.Quantity, h.Quote.Price)
	}
}
func finish(r Result, success int) (Result, error) {
	if success == 0 {
		return r, fmt.Errorf("%s", strings.Join(r.Warnings, "；"))
	}
	return r, nil
}
func warn(r *Result, label string, err error) {
	if err != nil {
		r.Warnings = append(r.Warnings, label+"："+err.Error())
	}
}
