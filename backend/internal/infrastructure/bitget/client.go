// Package bitget reads public stock-token and Quanto market data. It never
// needs trading credentials and keeps exchange products separate from equities.
package bitget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Instrument struct {
	StockCode  string
	StockName  string
	Symbol     string `json:"symbol"`
	BaseCoin   string `json:"baseCoin"`
	QuoteCoin  string `json:"quoteCoin"`
	SymbolType string `json:"symbolType"`
	IsReality  string `json:"isReality"`
	Status     string `json:"status"`
}

type Ticker struct {
	Symbol    string `json:"symbol"`
	Last      string `json:"lastPrice"`
	Open      string `json:"openPrice24h"`
	High      string `json:"highPrice24h"`
	Low       string `json:"lowPrice24h"`
	Change    string `json:"price24hPcnt"`
	Volume    string `json:"volume24h"`
	Timestamp string `json:"ts"`
}

type Quote struct {
	Code, Name, Market, ProviderSymbol, Currency, ProductType string
	Price, Open, High, Low, Change, ChangePercent, Volume     float64
	Timestamp                                                 string
}

type Candle struct {
	Time                           time.Time
	Open, High, Low, Close, Volume float64
}

type cachedMarket struct {
	mu             sync.Mutex
	Instruments    []Instrument
	InstrumentTime time.Time
	Tickers        map[string]Ticker
	TickerTime     time.Time
}

func (c *Client) marketCache(market string) *cachedMarket {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry := c.markets[market]; entry != nil {
		return entry
	}
	entry := &cachedMarket{}
	c.markets[market] = entry
	return entry
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	mu         sync.Mutex
	markets    map[string]*cachedMarket
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.bitget.com"
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: &http.Client{Timeout: 3 * time.Second}, markets: make(map[string]*cachedMarket)}
}

func (c *Client) get(ctx context.Context, path string, params url.Values, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("Bitget request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bitget HTTP %d", resp.StatusCode)
	}
	var result struct {
		Code string          `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&result); err != nil {
		return fmt.Errorf("invalid Bitget response")
	}
	if result.Code != "00000" {
		return fmt.Errorf("Bitget error %s", result.Code)
	}
	return json.Unmarshal(result.Data, target)
}

func category(market string) string {
	if market == "hk_stock" {
		return "USDT-FUTURES"
	}
	return "SPOT"
}

// Instruments is also the authoritative discovery list: no unlisted synthetic
// symbol is constructed, and offline contracts are not exposed in search.
func (c *Client) Instruments(ctx context.Context, market string) ([]Instrument, error) {
	if market != "us_stock" && market != "hk_stock" {
		return nil, fmt.Errorf("unsupported Bitget market")
	}
	entry := c.marketCache(market)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if len(entry.Instruments) > 0 && time.Since(entry.InstrumentTime) < 15*time.Minute {
		return entry.Instruments, nil
	}
	var all []Instrument
	if err := c.get(ctx, "/api/v3/market/instruments", url.Values{"category": {category(market)}}, &all); err != nil {
		return nil, err
	}
	rows := make([]Instrument, 0)
	for _, item := range all {
		if item.SymbolType != "stock" || item.Status == "offline" || item.Status == "listed" {
			continue
		}
		if market == "hk_stock" && strings.HasSuffix(item.BaseCoin, "HKD") {
			rows = append(rows, item)
		}
		if market == "us_stock" && item.IsReality == "yes" {
			rows = append(rows, item)
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Bitget has no listed stock instruments")
	}
	if market == "us_stock" {
		var info []struct {
			Symbol string `json:"symbol"`
			Code   string `json:"code"`
			Name   string `json:"name"`
		}
		infoCtx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
		if c.get(infoCtx, "/api/v3/reality/market/stock-info", nil, &info) == nil {
			bySymbol := make(map[string]int, len(rows))
			for i, row := range rows {
				bySymbol[row.Symbol] = i
			}
			for _, stock := range info {
				if i, ok := bySymbol[stock.Symbol]; ok {
					rows[i].StockCode = stock.Code
					rows[i].StockName = stock.Name
				}
			}
		}
		cancel()
	}
	entry.Instruments = rows
	entry.InstrumentTime = time.Now()
	return rows, nil
}

func (c *Client) tickers(ctx context.Context, market string) (map[string]Ticker, error) {
	entry := c.marketCache(market)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if len(entry.Tickers) > 0 && time.Since(entry.TickerTime) < 3*time.Second {
		return entry.Tickers, nil
	}
	var rows []Ticker
	if err := c.get(ctx, "/api/v3/market/tickers", url.Values{"category": {category(market)}}, &rows); err != nil {
		return nil, err
	}
	values := make(map[string]Ticker, len(rows))
	for _, row := range rows {
		values[row.Symbol] = row
	}
	entry.Tickers = values
	entry.TickerTime = time.Now()
	return values, nil
}

var stockNames = map[string]string{
	"AAPL": "苹果 Apple", "MSFT": "微软 Microsoft", "NVDA": "英伟达 NVIDIA", "TSLA": "特斯拉 Tesla", "AMZN": "亚马逊 Amazon", "GOOGL": "谷歌 Alphabet", "GOOG": "谷歌 Alphabet", "META": "Meta", "AMD": "超微 AMD", "NFLX": "奈飞 Netflix", "COIN": "Coinbase", "MSTR": "Strategy", "HOOD": "Robinhood", "QQQ": "纳斯达克100 ETF", "SPY": "标普500 ETF", "GLD": "黄金 ETF", "BABA": "阿里巴巴 ADR", "JD": "京东 ADR", "PDD": "拼多多 ADR", "CRCL": "Circle", "TSM": "台积电 ADR", "INTC": "英特尔 Intel", "AVGO": "博通 Broadcom", "PLTR": "Palantir", "ORCL": "甲骨文 Oracle", "JPM": "摩根大通", "V": "Visa", "WMT": "沃尔玛", "KO": "可口可乐", "NKE": "耐克 Nike", "DIS": "迪士尼 Disney",
}

var hkStocks = map[string]struct{ Code, Name string }{
	"TENCENTHKD": {"00700", "腾讯控股 Tencent"},
	"XIAOMIHKD":  {"01810", "小米集团 Xiaomi"},
	"LENOVOHKD":  {"00992", "联想集团 Lenovo"},
	"MINIMAXHKD": {"00100", "MiniMax"},
	"ZHIPUHKD":   {"02513", "智谱 Zhipu"},
}

func identity(item Instrument, market string) (code, name, currency, product string) {
	if market == "hk_stock" {
		if known, ok := hkStocks[item.BaseCoin]; ok {
			return known.Code, known.Name, "HKD", "quanto_perpetual"
		}
		code = strings.TrimSuffix(item.BaseCoin, "HKD")
		return code, code, "HKD", "quanto_perpetual"
	}
	code = strings.TrimPrefix(strings.ToUpper(item.BaseCoin), "R")
	if item.StockCode != "" {
		code = item.StockCode
	}
	name = stockNames[code]
	if name == "" {
		name = item.StockName
	}
	if name == "" {
		name = code
	}
	return code, name, item.QuoteCoin, "stock_token"
}

func NormalizeCode(market, code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if market == "hk_stock" {
		code = strings.TrimSuffix(strings.TrimPrefix(code, "HK"), ".HK")
		if n, err := strconv.Atoi(code); err == nil && n > 0 && n <= 99999 {
			return fmt.Sprintf("%05d", n)
		}
	}
	return code
}

func number(v string) float64 { n, _ := strconv.ParseFloat(v, 64); return n }

func toQuote(item Instrument, ticker Ticker, market string) Quote {
	code, name, currency, product := identity(item, market)
	price, open := number(ticker.Last), number(ticker.Open)
	return Quote{Code: code, Name: name, Market: market, ProviderSymbol: item.Symbol, Currency: currency, ProductType: product, Price: price, Open: open, High: number(ticker.High), Low: number(ticker.Low), Change: price - open, ChangePercent: number(ticker.Change) * 100, Volume: number(ticker.Volume), Timestamp: ticker.Timestamp}
}

func (c *Client) Quotes(ctx context.Context, market string, codes []string) ([]Quote, error) {
	instruments, err := c.Instruments(ctx, market)
	if err != nil {
		return nil, err
	}
	tickers, err := c.tickers(ctx, market)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool)
	for _, code := range codes {
		wanted[NormalizeCode(market, code)] = true
	}
	rows := make([]Quote, 0, len(codes))
	for _, item := range instruments {
		code, _, _, _ := identity(item, market)
		if !wanted[code] && !wanted[item.Symbol] {
			continue
		}
		if ticker, ok := tickers[item.Symbol]; ok && number(ticker.Last) > 0 {
			rows = append(rows, toQuote(item, ticker, market))
		}
	}
	return rows, nil
}

func (c *Client) Search(ctx context.Context, market, query string) ([]Quote, error) {
	instruments, err := c.Instruments(ctx, market)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	defaults := map[string]bool{"AAPL": true, "NVDA": true, "TSLA": true, "MSFT": true, "GOOGL": true, "AMZN": true, "META": true, "AMD": true, "NFLX": true, "QQQ": true, "SPY": true}
	var matched []Instrument
	for _, item := range instruments {
		code, name, _, _ := identity(item, market)
		if q == "" {
			if market == "hk_stock" || defaults[code] {
				matched = append(matched, item)
			}
		} else if strings.Contains(strings.ToLower(code+" "+name+" "+item.Symbol), q) || NormalizeCode(market, query) == code {
			matched = append(matched, item)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		a, _, _, _ := identity(matched[i], market)
		b, _, _, _ := identity(matched[j], market)
		if strings.EqualFold(a, query) {
			return !strings.EqualFold(b, query)
		}
		return a < b
	})
	if len(matched) > 50 {
		matched = matched[:50]
	}
	if len(matched) == 0 {
		return []Quote{}, nil
	}
	tickers, err := c.tickers(ctx, market)
	if err != nil {
		return nil, err
	}
	rows := make([]Quote, 0, len(matched))
	for _, item := range matched {
		if t, ok := tickers[item.Symbol]; ok && number(t.Last) > 0 {
			rows = append(rows, toQuote(item, t, market))
		}
	}
	return rows, nil
}

func (c *Client) Candles(ctx context.Context, market, code, period string, limit int) ([]Candle, error) {
	items, err := c.Instruments(ctx, market)
	if err != nil {
		return nil, err
	}
	var symbol string
	for _, item := range items {
		id, _, _, _ := identity(item, market)
		if id == NormalizeCode(market, code) || item.Symbol == strings.ToUpper(code) {
			symbol = item.Symbol
			break
		}
	}
	if symbol == "" {
		return nil, fmt.Errorf("stock is not listed on Bitget")
	}
	intervals := map[string]string{"5m": "5m", "15m": "15m", "30m": "15m", "1h": "1H", "4h": "4H", "1d": "1D", "1w": "1D"}
	interval, ok := intervals[period]
	if !ok {
		return nil, fmt.Errorf("unsupported interval")
	}
	if limit < 1 {
		limit = 150
	}
	if limit > 1000 {
		limit = 1000
	}
	need := limit
	if period == "30m" {
		need = limit*2 + 2
	}
	if period == "1w" {
		need = limit*7 + 7
	}
	all := make(map[int64]Candle)
	end := ""
	for len(all) < need {
		n := need - len(all)
		path := "/api/v3/market/candles"
		max := 1000
		if end != "" {
			path = "/api/v3/market/history-candles"
			max = 200
		}
		if n > max {
			n = max
		}
		params := url.Values{"category": {category(market)}, "symbol": {symbol}, "interval": {interval}, "type": {"market"}, "limit": {strconv.Itoa(n)}}
		if end != "" {
			params.Set("endTime", end)
		}
		var raw [][]string
		if err := c.get(ctx, path, params, &raw); err != nil {
			if len(all) == 0 {
				return nil, err
			}
			break
		}
		if len(raw) == 0 {
			break
		}
		oldCount := len(all)
		earliest := int64(1<<63 - 1)
		for _, row := range raw {
			if len(row) < 6 {
				continue
			}
			ts, err := strconv.ParseInt(row[0], 10, 64)
			if err != nil {
				continue
			}
			if ts < earliest {
				earliest = ts
			}
			all[ts] = Candle{Time: time.UnixMilli(ts).UTC(), Open: number(row[1]), High: number(row[2]), Low: number(row[3]), Close: number(row[4]), Volume: number(row[5])}
		}
		if len(all) == oldCount || len(raw) < n {
			break
		}
		end = strconv.FormatInt(earliest-1, 10)
	}
	rows := make([]Candle, 0, len(all))
	for _, row := range all {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time.Before(rows[j].Time) })
	if period == "30m" || period == "1w" {
		rows = Aggregate(rows, period)
	}
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Bitget returned no candles")
	}
	return rows, nil
}

func Aggregate(rows []Candle, period string) []Candle {
	out := make([]Candle, 0)
	for _, row := range rows {
		start := row.Time.Truncate(30 * time.Minute)
		if period == "1w" {
			d := row.Time.UTC()
			start = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(int(d.Weekday())+6)%7)
		}
		if len(out) == 0 || !out[len(out)-1].Time.Equal(start) {
			row.Time = start
			out = append(out, row)
			continue
		}
		last := &out[len(out)-1]
		if row.High > last.High {
			last.High = row.High
		}
		if row.Low < last.Low {
			last.Low = row.Low
		}
		last.Close = row.Close
		last.Volume += row.Volume
	}
	return out
}
