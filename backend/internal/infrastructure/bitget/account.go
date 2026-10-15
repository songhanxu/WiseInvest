package bitget

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Credentials struct {
	APIKey     string `json:"-"`
	Secret     string `json:"-"`
	Passphrase string `json:"-"`
}
type AccountClient struct {
	BaseURL     string
	HTTPClient  *http.Client
	credentials Credentials
	Now         func() time.Time
}

func NewAccountClient(credentials Credentials) *AccountClient {
	return &AccountClient{BaseURL: "https://api.bitget.com", HTTPClient: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, credentials: credentials, Now: time.Now}
}

// PrivateGET deliberately permits only documented read endpoints. It never
// sends credentials in a URL or follows redirects with authentication headers.
func (c *AccountClient) PrivateGET(ctx context.Context, path string, params url.Values, target interface{}) error {
	allowed := map[string]bool{"/api/v3/account/assets": true, "/api/v2/spot/account/assets": true, "/api/v3/position/current-position": true, "/api/v2/mix/position/all-position": true}
	if !allowed[path] {
		return fmt.Errorf("unsupported read-only account endpoint")
	}
	if c.credentials.APIKey == "" || c.credentials.Secret == "" || c.credentials.Passphrase == "" {
		return fmt.Errorf("Bitget account credentials are incomplete")
	}
	query := params.Encode()
	requestPath := path
	if query != "" {
		requestPath += "?" + query
	}
	timestamp := strconv.FormatInt(c.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(c.credentials.Secret))
	mac.Write([]byte(timestamp + http.MethodGet + requestPath))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+requestPath, nil)
	if err != nil {
		return fmt.Errorf("invalid Bitget account request")
	}
	req.Header.Set("ACCESS-KEY", c.credentials.APIKey)
	req.Header.Set("ACCESS-PASSPHRASE", c.credentials.Passphrase)
	req.Header.Set("ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("ACCESS-SIGN", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return accountConnectionError(err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Code string          `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope) != nil {
		kind := "response"
		if resp.StatusCode != http.StatusOK {
			kind = "http"
		}
		return &AccountError{HTTPStatus: resp.StatusCode, Kind: kind}
	}
	if envelope.Code != "00000" {
		// Never relay upstream messages, which can contain account identifiers.
		code := ""
		if safeAccountCode(envelope.Code) {
			code = envelope.Code
		}
		return &AccountError{Code: code, HTTPStatus: resp.StatusCode, Kind: "rejected"}
	}
	if resp.StatusCode != http.StatusOK {
		return &AccountError{HTTPStatus: resp.StatusCode, Kind: "http"}
	}
	if string(envelope.Data) == "null" || len(envelope.Data) == 0 {
		return &AccountError{Kind: "response"}
	}
	if json.Unmarshal(envelope.Data, target) != nil {
		return &AccountError{Kind: "response"}
	}
	return nil
}

type Asset struct {
	Coin      string `json:"coin"`
	Balance   string `json:"balance"`
	Equity    string `json:"equity"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Locked    string `json:"locked"`
}

type Position struct {
	EntryPrice        string `json:"avgPrice"`
	ClassicEntryPrice string `json:"openPriceAvg"`
	UnrealizedPnL     string `json:"unrealisedPnl"`
	ClassicPnL        string `json:"unrealizedPL"`
	MarginCoin        string `json:"marginCoin"`
	Symbol            string `json:"symbol"`
	Total             string `json:"total"`
	Side              string `json:"posSide"`
	ClassicSide       string `json:"holdSide"`
	MarginMode        string `json:"marginMode"`
}

func (c *AccountClient) Assets(ctx context.Context, accountType string) ([]Asset, error) {
	if accountType == "classic" {
		var assets []Asset
		err := c.PrivateGET(ctx, "/api/v2/spot/account/assets", nil, &assets)
		return assets, err
	}
	var data struct {
		Assets *[]Asset `json:"assets"`
	}
	err := c.PrivateGET(ctx, "/api/v3/account/assets", nil, &data)
	if err != nil {
		return nil, err
	}
	if data.Assets == nil {
		return nil, &AccountError{Kind: "response"}
	}
	return *data.Assets, nil
}

func (c *AccountClient) Positions(ctx context.Context, accountType string) ([]Position, error) {
	if accountType == "classic" {
		var positions []Position
		err := c.PrivateGET(ctx, "/api/v2/mix/position/all-position", url.Values{"productType": {"USDT-FUTURES"}}, &positions)
		for i := range positions {
			positions[i].Side = positions[i].ClassicSide
			positions[i].EntryPrice = positions[i].ClassicEntryPrice
			positions[i].UnrealizedPnL = positions[i].ClassicPnL
		}
		return positions, err
	}
	var data struct {
		List *[]Position `json:"list"`
	}
	err := c.PrivateGET(ctx, "/api/v3/position/current-position", url.Values{"category": {"USDT-FUTURES"}}, &data)
	if err != nil {
		return nil, err
	}
	if data.List == nil {
		return nil, &AccountError{Kind: "response"}
	}
	return *data.List, nil
}

func PositiveQuantity(quantity string) bool {
	value, ok := new(big.Rat).SetString(quantity)
	return ok && value.Sign() > 0
}

func AssetQuantity(asset Asset, accountType string) string {
	if accountType != "classic" {
		if asset.Balance != "" {
			return asset.Balance
		}
		return asset.Equity
	}
	total := new(big.Rat)
	precision := 0
	for _, v := range []string{asset.Available, asset.Frozen, asset.Locked} {
		number, ok := new(big.Rat).SetString(v)
		if !ok {
			continue
		}
		total.Add(total, number)
		if point := strings.IndexByte(v, '.'); point >= 0 && len(v)-point-1 > precision {
			precision = len(v) - point - 1
		}
	}
	if precision > 18 {
		precision = 18
	}
	value := total.FloatString(precision)
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	return value
}

// Inventory includes legacy stock tokens and listed stock perpetuals as well
// as Reality tokens. Crypto assets are identified by instrument metadata, not
// by guessing that any coin beginning with R must be a stock.
func (c *Client) StockInventory(ctx context.Context, productCategory string) ([]Instrument, error) {
	if productCategory != "SPOT" && productCategory != "USDT-FUTURES" {
		return nil, fmt.Errorf("unsupported stock category")
	}
	entry := c.marketCache("account:" + productCategory)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if len(entry.Instruments) > 0 && time.Since(entry.InstrumentTime) < 15*time.Minute {
		return entry.Instruments, nil
	}
	var all []Instrument
	if err := c.get(ctx, "/api/v3/market/instruments", url.Values{"category": {productCategory}}, &all); err != nil {
		return nil, err
	}
	rows := make([]Instrument, 0)
	for _, item := range all {
		if item.SymbolType == "stock" {
			rows = append(rows, item)
		}
	}
	entry.Instruments = rows
	entry.InstrumentTime = time.Now()
	return rows, nil
}

func (c *Client) InventoryTickers(ctx context.Context, productCategory string) (map[string]Ticker, error) {
	market := "us_stock"
	if productCategory == "USDT-FUTURES" {
		market = "hk_stock"
	}
	return c.tickers(ctx, market)
}

func HoldingIdentity(item Instrument, productCategory string) Quote {
	market := "us_stock"
	currency := item.QuoteCoin
	product := "stock_token"
	code, name, _, _ := identity(item, "us_stock")
	// Reality's prefix must not strip a genuine leading R from legacy tokens.
	if productCategory == "SPOT" && item.IsReality != "yes" && !strings.HasPrefix(item.BaseCoin, "r") && item.StockCode == "" {
		code = strings.ToUpper(item.BaseCoin)
		underlying := strings.TrimSuffix(code, "X")
		if stockNames[underlying] != "" {
			code = underlying
		}
		name = stockNames[code]
		if name == "" {
			name = item.StockName
		}
		if name == "" {
			name = code
		}
	}
	if productCategory == "USDT-FUTURES" {
		product = "stock_perpetual"
		code = item.BaseCoin
		name = stockNames[code]
		if name == "" {
			name = code
		}
		if strings.HasSuffix(item.BaseCoin, "HKD") {
			market = "hk_stock"
			code, name, currency, product = identity(item, market)
		}
		if strings.HasSuffix(item.BaseCoin, "JPY") {
			market = "jp_stock"
			currency = "JPY"
			code = strings.TrimSuffix(item.BaseCoin, "JPY")
			name = code
			product = "quanto_perpetual"
		}
	}
	return Quote{Code: code, Name: name, Market: market, ProviderSymbol: item.Symbol, Currency: currency, ProductType: product}
}

func ApplyTicker(quote Quote, ticker Ticker) Quote {
	quote.Price = number(ticker.Last)
	quote.Open = number(ticker.Open)
	quote.High = number(ticker.High)
	quote.Low = number(ticker.Low)
	quote.Change = quote.Price - quote.Open
	quote.ChangePercent = number(ticker.Change) * 100
	quote.Volume = number(ticker.Volume)
	quote.Timestamp = ticker.Timestamp
	return quote
}

func QuantityValue(quantity string, price float64) string {
	q, ok := new(big.Rat).SetString(quantity)
	if !ok || price <= 0 {
		return ""
	}
	p, ok := new(big.Rat).SetString(strconv.FormatFloat(price, 'f', -1, 64))
	if !ok {
		return ""
	}
	return new(big.Rat).Mul(q, p).FloatString(8)
}
