package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	BaseURL = "https://api.binance.com"
	
	// API endpoints
	EndpointAccount      = "/api/v3/account"
	EndpointOrder        = "/api/v3/order"
	EndpointOpenOrders   = "/api/v3/openOrders"
	EndpointKlines       = "/api/v3/klines"
	EndpointTicker24hr   = "/api/v3/ticker/24hr"
	EndpointTickerPrice  = "/api/v3/ticker/price"
)

// Client represents a Binance API client
type Client struct {
	apiKey    string
	apiSecret string
	baseURL   string
	httpClient *http.Client
}

// NewClient creates a new Binance client
func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   BaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AccountInfo represents account information
type AccountInfo struct {
	MakerCommission  int       `json:"makerCommission"`
	TakerCommission  int       `json:"takerCommission"`
	BuyerCommission  int       `json:"buyerCommission"`
	SellerCommission int       `json:"sellerCommission"`
	CanTrade         bool      `json:"canTrade"`
	CanWithdraw      bool      `json:"canWithdraw"`
	CanDeposit       bool      `json:"canDeposit"`
	UpdateTime       int64     `json:"updateTime"`
	AccountType      string    `json:"accountType"`
	Balances         []Balance `json:"balances"`
	Permissions      []string  `json:"permissions"`
}

// Balance represents asset balance
type Balance struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
}

// OrderRequest represents an order request
type OrderRequest struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`              // BUY or SELL
	Type             string  `json:"type"`              // LIMIT, MARKET, etc.
	TimeInForce      string  `json:"timeInForce,omitempty"`
	Quantity         float64 `json:"quantity,omitempty"`
	QuoteOrderQty    float64 `json:"quoteOrderQty,omitempty"`
	Price            float64 `json:"price,omitempty"`
	StopPrice        float64 `json:"stopPrice,omitempty"`
	IcebergQty       float64 `json:"icebergQty,omitempty"`
	NewOrderRespType string  `json:"newOrderRespType,omitempty"`
}

// OrderResponse represents an order response
type OrderResponse struct {
	Symbol              string `json:"symbol"`
	OrderID             int64  `json:"orderId"`
	ClientOrderID       string `json:"clientOrderId"`
	TransactTime        int64  `json:"transactTime"`
	Price               string `json:"price"`
	OrigQty             string `json:"origQty"`
	ExecutedQty         string `json:"executedQty"`
	CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
	Status              string `json:"status"`
	TimeInForce         string `json:"timeInForce"`
	Type                string `json:"type"`
	Side                string `json:"side"`
}

// Kline represents candlestick data
type Kline struct {
	OpenTime                 int64
	Open                     string
	High                     string
	Low                      string
	Close                    string
	Volume                   string
	CloseTime                int64
	QuoteAssetVolume         string
	NumberOfTrades           int
	TakerBuyBaseAssetVolume  string
	TakerBuyQuoteAssetVolume string
}

// Ticker24hr represents 24hr ticker price change statistics
type Ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	WeightedAvgPrice   string `json:"weightedAvgPrice"`
	PrevClosePrice     string `json:"prevClosePrice"`
	LastPrice          string `json:"lastPrice"`
	LastQty            string `json:"lastQty"`
	BidPrice           string `json:"bidPrice"`
	AskPrice           string `json:"askPrice"`
	OpenPrice          string `json:"openPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	OpenTime           int64  `json:"openTime"`
	CloseTime          int64  `json:"closeTime"`
	Count              int    `json:"count"`
}

// GetAccountInfo gets account information
func (c *Client) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	params := url.Values{}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	resp, err := c.doSignedRequest(ctx, "GET", EndpointAccount, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var accountInfo AccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&accountInfo); err != nil {
		return nil, fmt.Errorf("failed to decode account info: %w", err)
	}

	return &accountInfo, nil
}

// CreateOrder creates a new order
func (c *Client) CreateOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	params := url.Values{}
	params.Set("symbol", req.Symbol)
	params.Set("side", req.Side)
	params.Set("type", req.Type)
	
	if req.TimeInForce != "" {
		params.Set("timeInForce", req.TimeInForce)
	}
	if req.Quantity > 0 {
		params.Set("quantity", fmt.Sprintf("%.8f", req.Quantity))
	}
	if req.QuoteOrderQty > 0 {
		params.Set("quoteOrderQty", fmt.Sprintf("%.8f", req.QuoteOrderQty))
	}
	if req.Price > 0 {
		params.Set("price", fmt.Sprintf("%.8f", req.Price))
	}
	if req.StopPrice > 0 {
		params.Set("stopPrice", fmt.Sprintf("%.8f", req.StopPrice))
	}
	
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	resp, err := c.doSignedRequest(ctx, "POST", EndpointOrder, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return nil, fmt.Errorf("failed to decode order response: %w", err)
	}

	return &orderResp, nil
}

// GetOpenOrders gets all open orders
func (c *Client) GetOpenOrders(ctx context.Context, symbol string) ([]OrderResponse, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", symbol)
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	resp, err := c.doSignedRequest(ctx, "GET", EndpointOpenOrders, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var orders []OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %w", err)
	}

	return orders, nil
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, symbol string, orderID int64) (*OrderResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("orderId", strconv.FormatInt(orderID, 10))
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	resp, err := c.doSignedRequest(ctx, "DELETE", EndpointOrder, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return nil, fmt.Errorf("failed to decode order response: %w", err)
	}

	return &orderResp, nil
}

// GetKlines gets candlestick data
func (c *Client) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]Kline, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	resp, err := c.doRequest(ctx, "GET", EndpointKlines, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rawKlines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawKlines); err != nil {
		return nil, fmt.Errorf("failed to decode klines: %w", err)
	}

	klines := make([]Kline, len(rawKlines))
	for i, k := range rawKlines {
		klines[i] = Kline{
			OpenTime:                 int64(k[0].(float64)),
			Open:                     k[1].(string),
			High:                     k[2].(string),
			Low:                      k[3].(string),
			Close:                    k[4].(string),
			Volume:                   k[5].(string),
			CloseTime:                int64(k[6].(float64)),
			QuoteAssetVolume:         k[7].(string),
			NumberOfTrades:           int(k[8].(float64)),
			TakerBuyBaseAssetVolume:  k[9].(string),
			TakerBuyQuoteAssetVolume: k[10].(string),
		}
	}

	return klines, nil
}

// Get24hrTicker gets 24hr ticker price change statistics
func (c *Client) Get24hrTicker(ctx context.Context, symbol string) (*Ticker24hr, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	resp, err := c.doRequest(ctx, "GET", EndpointTicker24hr, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ticker Ticker24hr
	if err := json.NewDecoder(resp.Body).Decode(&ticker); err != nil {
		return nil, fmt.Errorf("failed to decode ticker: %w", err)
	}

	return &ticker, nil
}

// doRequest performs an unsigned request
func (c *Client) doRequest(ctx context.Context, method, endpoint string, params url.Values) (*http.Response, error) {
	reqURL := c.baseURL + endpoint
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-MBX-APIKEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return resp, nil
}

// doSignedRequest performs a signed request
func (c *Client) doSignedRequest(ctx context.Context, method, endpoint string, params url.Values) (*http.Response, error) {
	// Add signature
	signature := c.sign(params.Encode())
	params.Set("signature", signature)

	return c.doRequest(ctx, method, endpoint, params)
}

// sign creates HMAC SHA256 signature
func (c *Client) sign(message string) string {
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
