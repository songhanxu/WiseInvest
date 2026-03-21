package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// StockHandler provides real-time market data endpoints.
type StockHandler struct {
	watchlistRepo *repository.WatchlistRepository
	logger        *logger.Logger
	httpClient    *http.Client
	cache         *cache.QuoteCache // Redis-backed quote cache (may be nil if cache is disabled)
	stopTicker    context.CancelFunc
}

// NewStockHandler creates a new StockHandler.
// If quoteCache is non-nil, background tickers will be started to pre-populate
// Redis with fresh market data; HTTP handlers will read from the cache first.
func NewStockHandler(watchlistRepo *repository.WatchlistRepository, logger *logger.Logger, quoteCache *cache.QuoteCache) *StockHandler {
	h := &StockHandler{
		watchlistRepo: watchlistRepo,
		logger:        logger,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		cache:         quoteCache,
	}
	if quoteCache != nil {
		ctx, cancel := context.WithCancel(context.Background())
		h.stopTicker = cancel
		go h.backgroundRefreshLoop(ctx)
	}
	return h
}

// Stop gracefully shuts down background tickers.
func (h *StockHandler) Stop() {
	if h.stopTicker != nil {
		h.stopTicker()
	}
}

// backgroundRefreshLoop runs parallel tickers that continuously fetch market
// data from upstream APIs and write the results into Redis.
// Tick intervals are chosen to match exchange update frequencies:
//   - A-share indices:  ~1 s during trading hours
//   - US-stock indices: ~2 s during trading hours
//   - Crypto indices:   ~2 s (24/7)
func (h *StockHandler) backgroundRefreshLoop(ctx context.Context) {
	// Helper: run fn immediately then every interval until ctx is done.
	tick := func(name string, interval time.Duration, fn func()) {
		fn() // run once on startup
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fn()
			}
		}
	}

	// A-share indices — 1 s
	go tick("a_share_indices", 1*time.Second, func() {
		data, err := h.fetchAShareIndices(ctx)
		if err != nil || len(data) == 0 {
			return
		}
		_ = h.cache.SetJSON(ctx, cache.IndicesKey("a_share"), data, 5*time.Second)
	})

	// US-stock indices — 2 s
	go tick("us_stock_indices", 2*time.Second, func() {
		data, err := h.fetchUSStockIndices(ctx)
		if err != nil || len(data) == 0 {
			return
		}
		_ = h.cache.SetJSON(ctx, cache.IndicesKey("us_stock"), data, 10*time.Second)
	})

	// Crypto indices — 2 s
	go tick("crypto_indices", 2*time.Second, func() {
		data, err := h.fetchCryptoIndices(ctx)
		if err != nil || len(data) == 0 {
			return
		}
		_ = h.cache.SetJSON(ctx, cache.IndicesKey("crypto"), data, 10*time.Second)
	})

	// Block until context cancelled
	<-ctx.Done()
}

// ──────────────────────────────────────────────────────────────────────────────
// Market Indices — GET /api/v1/stocks/indices?market=a_share
// ──────────────────────────────────────────────────────────────────────────────

type IndexResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ShortName     string    `json:"short_name"`
	Value         float64   `json:"value"`
	Change        float64   `json:"change"`
	ChangePercent float64   `json:"change_percent"`
	SparklineData []float64 `json:"sparkline_data"`
}

func (h *StockHandler) GetIndices(c *gin.Context) {
	market := c.DefaultQuery("market", "a_share")

	// ── Try cache first (populated by background ticker) ─────────────────
	if h.cache != nil {
		cacheKey := cache.IndicesKey(market)
		var cached []IndexResponse
		if hit, _ := h.cache.GetJSON(c.Request.Context(), cacheKey, &cached); hit && len(cached) > 0 {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// ── Cache miss — fall back to real-time fetch ─────────────────────────
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	var indices []IndexResponse
	var err error

	switch market {
	case "a_share":
		indices, err = h.fetchAShareIndices(ctx)
	case "us_stock":
		indices, err = h.fetchUSStockIndices(ctx)
	case "crypto":
		indices, err = h.fetchCryptoIndices(ctx)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid market type"})
		return
	}

	if err != nil {
		h.logger.WithField("market", market).WithField("error", err).Warn("Failed to fetch indices, returning empty")
	}
	// Ensure we never return null — always return [] for JSON
	if indices == nil {
		indices = []IndexResponse{}
	}

	// Write to cache for next request
	if h.cache != nil && len(indices) > 0 {
		_ = h.cache.SetJSON(c.Request.Context(), cache.IndicesKey(market), indices, 5*time.Second)
	}

	c.JSON(http.StatusOK, indices)
}

// ── A-Share Indices (Tencent Finance API) ────────────────────────────────────

func (h *StockHandler) fetchAShareIndices(ctx context.Context) ([]IndexResponse, error) {
	codes := "sh000001,sz399001,sz399006" // 上证, 深证, 创业板
	return h.fetchTencentQuotes(ctx, codes, map[string]string{
		"sh000001": "上证",
		"sz399001": "深证",
		"sz399006": "创业板",
	})
}

func (h *StockHandler) fetchTencentQuotes(ctx context.Context, codes string, shortNames map[string]string) ([]IndexResponse, error) {
	apiURL := "http://qt.gtimg.cn/q=" + codes
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://finance.qq.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	utf8Reader := transform.NewReader(resp.Body, simplifiedchinese.GBK.NewDecoder())
	body, err := io.ReadAll(utf8Reader)
	if err != nil {
		return nil, err
	}

	var results []IndexResponse
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		start := strings.Index(line, "\"")
		end := strings.LastIndex(line, "\"")
		if start == -1 || end <= start {
			continue
		}
		fields := strings.Split(line[start+1:end], "~")
		if len(fields) < 33 {
			continue
		}
		name := fields[1]
		if name == "" {
			continue
		}

		// Extract the code key from the variable name (e.g., "v_sh000001")
		varPart := line[:start-1]
		codeKey := ""
		if idx := strings.LastIndex(varPart, "_"); idx >= 0 {
			codeKey = varPart[idx+1:]
		}

		price := parseFloat(fields[3])
		prevClose := parseFloat(fields[4])
		if price == 0 {
			price = prevClose
		}

		change := parseFloat(fields[31])
		changePct := parseFloat(fields[32])

		shortName := name
		if sn, ok := shortNames[codeKey]; ok {
			shortName = sn
		}

		results = append(results, IndexResponse{
			ID:            codeKey,
			Name:          name,
			ShortName:     shortName,
			Value:         price,
			Change:        change,
			ChangePercent: changePct,
			SparklineData: []float64{}, // Sparkline fetched separately if needed
		})
	}

	// Fetch sparkline data for each index in parallel (minute-level trend)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sparkline, err := h.fetchTencentMinuteData(ctx, results[idx].ID)
			if err == nil && len(sparkline) > 0 {
				results[idx].SparklineData = sparkline
			}
		}(i)
	}
	wg.Wait()

	return results, nil
}

// fetchMinuteData fetches minute-level price data for sparkline charts via Eastmoney.
func (h *StockHandler) fetchTencentMinuteData(ctx context.Context, code string) ([]float64, error) {
	// Convert tencent-style code (sh000001) to eastmoney secid (1.000001)
	secid := ""
	if strings.HasPrefix(code, "sh") {
		secid = "1." + code[2:]
	} else if strings.HasPrefix(code, "sz") {
		secid = "0." + code[2:]
	} else {
		return nil, fmt.Errorf("unknown code format: %s", code)
	}

	apiURL := fmt.Sprintf(
		"https://push2.eastmoney.com/api/qt/stock/trends2/get?secid=%s&fields1=f1,f2,f3,f4,f5,f6,f7,f8,f9,f10,f11,f12,f13&fields2=f51,f52,f53,f54,f55,f56,f57,f58&iscr=0&ndays=1&ut=bd1d9ddb04089700cf9c27f6f7426281",
		secid,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.eastmoney.com")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Trends []string `json:"trends"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var prices []float64
	for _, line := range result.Data.Trends {
		// Format: "2026-03-20 09:30,open,close,high,low,volume,amount,avg"
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			p := parseFloat(parts[2]) // close price
			if p > 0 {
				prices = append(prices, p)
			}
		}
	}

	// Downsample to ~20 points for sparkline
	if len(prices) > 20 {
		step := len(prices) / 20
		var sampled []float64
		for i := 0; i < len(prices); i += step {
			sampled = append(sampled, prices[i])
		}
		return sampled, nil
	}
	return prices, nil
}

// ── US Stock Indices (Yahoo Finance) ─────────────────────────────────────────

func (h *StockHandler) fetchUSStockIndices(ctx context.Context) ([]IndexResponse, error) {
	// Use Tencent Finance API (accessible from mainland China, Yahoo is blocked)
	indices := []struct {
		code      string // Tencent Finance code
		id        string
		name      string
		shortName string
	}{
		{"us.DJI", "DJI", "道琼斯工业平均指数", "道指"},
		{"us.INX", "GSPC", "标普500指数", "标普"},
		{"us.IXIC", "IXIC", "纳斯达克综合指数", "纳指"},
	}

	// Batch fetch all indices in one request
	codes := make([]string, len(indices))
	for i, idx := range indices {
		codes[i] = idx.code
	}
	apiURL := fmt.Sprintf("https://qt.gtimg.cn/q=%s", strings.Join(codes, ","))
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithField("error", err).Warn("fetchUSStockIndices: HTTP request failed")
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	h.logger.Debugf("fetchUSStockIndices: got %d bytes from Tencent Finance", len(body))
	// Tencent Finance returns GBK encoding, convert to UTF-8
	utf8Body, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), body)
	if err != nil {
		utf8Body = body // fallback
	}
	bodyStr := string(utf8Body)

	var results []IndexResponse
	// codeForSparkline tracks the Tencent code for each result index (for parallel sparkline fetch)
	var sparklineCodes []string
	for _, idx := range indices {
		// Parse: v_us.DJI="200~道琼斯~.DJI~45577.47~46021.43~..."
		prefix := fmt.Sprintf("v_%s=\"", idx.code)
		start := strings.Index(bodyStr, prefix)
		if start == -1 {
			h.logger.Warnf("US index %s not found in response", idx.code)
			continue
		}
		start += len(prefix)
		end := strings.Index(bodyStr[start:], "\"")
		if end == -1 {
			continue
		}
		fields := strings.Split(bodyStr[start:start+end], "~")
		// Tencent US stock fields: [0]=status, [1]=name, [2]=code, [3]=currentPrice,
		// [4]=previousClose, [5]=open, [6]=volume, ..., [31]=change, [32]=changePercent,
		// [33]=high, [34]=low
		if len(fields) < 35 {
			h.logger.Warnf("US index %s insufficient fields: %d", idx.code, len(fields))
			continue
		}

		currentPrice := parseFloat(fields[3])
		previousClose := parseFloat(fields[4])
		change := parseFloat(fields[31])
		changePct := parseFloat(fields[32])

		results = append(results, IndexResponse{
			ID:            idx.id,
			Name:          idx.name,
			ShortName:     idx.shortName,
			Value:         currentPrice,
			Change:        change,
			ChangePercent: changePct,
			SparklineData: []float64{},
		})
		sparklineCodes = append(sparklineCodes, idx.code)
		_ = previousClose
	}

	// Fetch sparkline data for all US indices in parallel
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sparkline := h.fetchUSIndexSparkline(ctx, sparklineCodes[idx])
			if len(sparkline) > 0 {
				results[idx].SparklineData = sparkline
			}
		}(i)
	}
	wg.Wait()

	return results, nil
}

// fetchUSIndexSparkline fetches 5-day close prices for US index sparkline chart
func (h *StockHandler) fetchUSIndexSparkline(ctx context.Context, code string) []float64 {
	apiURL := fmt.Sprintf("https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=%s,day,,,5,qfq", code)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var payload struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}

	// Find the day data in the response
	for _, v := range payload.Data {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		dayData, ok := m["day"].([]interface{})
		if !ok {
			continue
		}
		var sparkline []float64
		for _, d := range dayData {
			row, ok := d.([]interface{})
			if !ok || len(row) < 3 {
				continue
			}
			// K-line row: [date, open, close, high, low, volume]
			var closePrice float64
			switch v := row[2].(type) {
			case string:
				closePrice = parseFloat(v)
			case float64:
				closePrice = v
			}
			if closePrice > 0 {
				sparkline = append(sparkline, closePrice)
			}
		}
		if len(sparkline) > 0 {
			return sparkline
		}
	}
	return nil
}

// ── Crypto Indices (Binance API) ─────────────────────────────────────────────

func (h *StockHandler) fetchCryptoIndices(ctx context.Context) ([]IndexResponse, error) {
	// Fetch BTC and ETH 24hr tickers from Binance for index display
	tickers, err := h.fetchBinance24hrTickers(ctx, []string{"BTCUSDT", "ETHUSDT"})
	if err != nil {
		return nil, err
	}

	var btcPrice, btcChangePct float64
	var ethPrice float64
	if t, ok := tickers["BTCUSDT"]; ok {
		btcPrice = parseFloat(t.LastPrice)
		btcChangePct = parseFloat(t.PriceChangePercent)
	}
	if t, ok := tickers["ETHUSDT"]; ok {
		ethPrice = parseFloat(t.LastPrice)
	}
	_ = ethPrice

	// Fetch sparkline data and fear/greed index in parallel
	var btcSparkline, ethSparkline []float64
	var fgIdx IndexResponse
	var fgErr error
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		btcSparkline = h.fetchCryptoSparkline(ctx, "BTCUSDT")
	}()
	go func() {
		defer wg.Done()
		ethSparkline = h.fetchCryptoSparkline(ctx, "ETHUSDT")
	}()
	go func() {
		defer wg.Done()
		fgIdx, fgErr = h.fetchFearGreedIndex(ctx)
	}()
	wg.Wait()

	indices := []IndexResponse{
		{
			ID:            "btc_price",
			Name:          "比特币",
			ShortName:     "BTC",
			Value:         btcPrice,
			Change:        parseFloat(tickers["BTCUSDT"].PriceChange),
			ChangePercent: btcChangePct,
			SparklineData: btcSparkline,
		},
	}

	if t, ok := tickers["ETHUSDT"]; ok {
		indices = append(indices, IndexResponse{
			ID:            "eth_price",
			Name:          "以太坊",
			ShortName:     "ETH",
			Value:         parseFloat(t.LastPrice),
			Change:        parseFloat(t.PriceChange),
			ChangePercent: parseFloat(t.PriceChangePercent),
			SparklineData: ethSparkline,
		})
	}

	// Fear & Greed index from alternative.me
	if fgErr == nil {
		indices = append(indices, fgIdx)
	}

	return indices, nil
}

// fetchCryptoSparkline fetches recent hourly close prices from Binance for sparkline display.
func (h *StockHandler) fetchCryptoSparkline(ctx context.Context, symbol string) []float64 {
	apiURL := fmt.Sprintf(
		"https://data-api.binance.vision/api/v3/klines?symbol=%s&interval=1h&limit=24",
		symbol,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return []float64{}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return []float64{}
	}
	defer resp.Body.Close()

	var rawKlines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawKlines); err != nil {
		return []float64{}
	}

	var prices []float64
	for _, k := range rawKlines {
		if len(k) < 5 {
			continue
		}
		if closeStr, ok := k[4].(string); ok {
			if p := parseFloat(closeStr); p > 0 {
				prices = append(prices, p)
			}
		}
	}
	return prices
}

// BinanceTicker24hr represents a Binance 24hr ticker response.
type BinanceTicker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	WeightedAvgPrice   string `json:"weightedAvgPrice"`
	PrevClosePrice     string `json:"prevClosePrice"`
	LastPrice          string `json:"lastPrice"`
	OpenPrice          string `json:"openPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
}

// fetchBinance24hrTickers fetches 24hr ticker data for given Binance symbols.
func (h *StockHandler) fetchBinance24hrTickers(ctx context.Context, symbols []string) (map[string]BinanceTicker24hr, error) {
	// Build symbols JSON array: ["BTCUSDT","ETHUSDT"]
	symbolsJSON := "[\"" + strings.Join(symbols, "\",\"") + "\"]"
	apiURL := fmt.Sprintf("https://data-api.binance.vision/api/v3/ticker/24hr?symbols=%s", url.QueryEscape(symbolsJSON))
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance API returned %d: %s", resp.StatusCode, string(body))
	}

	var tickers []BinanceTicker24hr
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, err
	}

	result := make(map[string]BinanceTicker24hr, len(tickers))
	for _, t := range tickers {
		result[t.Symbol] = t
	}
	return result, nil
}

func (h *StockHandler) fetchFearGreedIndex(ctx context.Context) (IndexResponse, error) {
	apiURL := "https://api.alternative.me/fng/?limit=2"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return IndexResponse{}, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return IndexResponse{}, err
	}
	defer resp.Body.Close()

	var data struct {
		Data []struct {
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return IndexResponse{}, err
	}

	if len(data.Data) == 0 {
		return IndexResponse{}, fmt.Errorf("no fear greed data")
	}

	current := parseFloat(data.Data[0].Value)
	prev := current
	if len(data.Data) > 1 {
		prev = parseFloat(data.Data[1].Value)
	}
	change := current - prev
	changePct := 0.0
	if prev > 0 {
		changePct = change / prev * 100
	}

	return IndexResponse{
		ID:            "fear_greed",
		Name:          "恐惧贪婪指数",
		ShortName:     "情绪",
		Value:         current,
		Change:        change,
		ChangePercent: changePct,
		SparklineData: []float64{},
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Stock Search — GET /api/v1/stocks/search?q=xxx&market=a_share
// ──────────────────────────────────────────────────────────────────────────────

type StockResponse struct {
	ID            string  `json:"id"`
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Market        string  `json:"market"`
	CurrentPrice  float64 `json:"current_price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Volume        float64 `json:"volume"` // in 亿
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Open          float64 `json:"open"`
	PreviousClose float64 `json:"previous_close"`
}

func (h *StockHandler) SearchStocks(c *gin.Context) {
	query := c.DefaultQuery("q", "")
	market := c.DefaultQuery("market", "a_share")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var stocks []StockResponse
	var err error

	switch market {
	case "a_share":
		stocks, err = h.searchAShareStocks(ctx, query)
	case "us_stock":
		stocks, err = h.searchUSStocks(ctx, query)
	case "crypto":
		stocks, err = h.searchCryptoStocks(ctx, query)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid market type"})
		return
	}

	if err != nil {
		h.logger.WithField("error", err).Warn("Stock search failed")
		stocks = []StockResponse{}
	}

	c.JSON(http.StatusOK, stocks)
}

// ── A-Share Search (Eastmoney) ───────────────────────────────────────────────

func (h *StockHandler) searchAShareStocks(ctx context.Context, query string) ([]StockResponse, error) {
	if query == "" {
		// Return popular A-share stocks
		return h.fetchAShareStocksByCode(ctx, "sh600519,sz000858,sh601318,sz300750,sz000001,sh600036,sz002594,sh600900,sz000333,sh603259")
	}

	// Use Eastmoney search API
	apiURL := "https://searchapi.eastmoney.com/api/suggest/get?input=" + url.QueryEscape(query) +
		"&type=14&count=10&markettype=&mktnum=&jys=&classify=&sectype="

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.eastmoney.com")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp struct {
		QuotationCodeTable struct {
			Data []struct {
				Code   string `json:"Code"`
				Name   string `json:"Name"`
				MktNum string `json:"MktNum"`
			} `json:"Data"`
		} `json:"QuotationCodeTable"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	// Build codes string for batch Tencent quote fetch
	var codes []string
	for _, item := range apiResp.QuotationCodeTable.Data {
		if len(item.Code) != 6 {
			continue
		}
		prefix := "sz"
		if item.MktNum == "1" {
			prefix = "sh"
		}
		codes = append(codes, prefix+item.Code)
	}

	if len(codes) == 0 {
		return []StockResponse{}, nil
	}

	return h.fetchAShareStocksByCode(ctx, strings.Join(codes, ","))
}

func (h *StockHandler) fetchAShareStocksByCode(ctx context.Context, codes string) ([]StockResponse, error) {
	apiURL := "http://qt.gtimg.cn/q=" + codes
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://finance.qq.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	utf8Reader := transform.NewReader(resp.Body, simplifiedchinese.GBK.NewDecoder())
	body, err := io.ReadAll(utf8Reader)
	if err != nil {
		return nil, err
	}

	var stocks []StockResponse
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		start := strings.Index(line, "\"")
		end := strings.LastIndex(line, "\"")
		if start == -1 || end <= start {
			continue
		}
		fields := strings.Split(line[start+1:end], "~")
		if len(fields) < 46 {
			continue
		}
		name := fields[1]
		if name == "" {
			continue
		}

		code := fields[2]
		price := parseFloat(fields[3])
		prevClose := parseFloat(fields[4])
		openPrice := parseFloat(fields[5])
		vol := parseFloat(fields[6])     // in 手
		high := parseFloat(fields[33])
		low := parseFloat(fields[34])

		if price == 0 {
			price = prevClose
		}

		changeAmt := parseFloat(fields[31])
		changePct := parseFloat(fields[32])

		// Determine symbol prefix
		prefix := "SH"
		if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			prefix = "SZ"
		}

		stocks = append(stocks, StockResponse{
			ID:            code,
			Symbol:        prefix + code,
			Name:          name,
			Market:        "a_share",
			CurrentPrice:  price,
			Change:        changeAmt,
			ChangePercent: changePct,
			Volume:        vol / 10000, // 手 → 万手 → 亿 (approximate: vol*100 shares / 1e8)
			High:          high,
			Low:           low,
			Open:          openPrice,
			PreviousClose: prevClose,
		})
	}
	return stocks, nil
}

// ── US Stock Search (Yahoo Finance) ──────────────────────────────────────────

func (h *StockHandler) searchUSStocks(ctx context.Context, query string) ([]StockResponse, error) {
	if query == "" {
		return h.fetchUSStocksBySymbol(ctx, "AAPL,NVDA,TSLA,MSFT,GOOGL,AMZN,META,AMD,NFLX")
	}

	// Tencent Finance smartbox search API (accessible from mainland China)
	apiURL := fmt.Sprintf("https://smartbox.gtimg.cn/s3/?q=%s&t=us", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// Response is GBK encoded
	utf8Body, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), body)
	if err != nil {
		utf8Body = body
	}
	bodyStr := string(utf8Body)

	// Parse: v_hint="us~aapl.oq~Apple~apple~GP^us~aple.n~apple hospitality reit, inc~*~GP"
	start := strings.Index(bodyStr, "\"")
	end := strings.LastIndex(bodyStr, "\"")
	if start == -1 || end <= start {
		return []StockResponse{}, nil
	}
	content := bodyStr[start+1 : end]

	var symbols []string
	entries := strings.Split(content, "^")
	for _, entry := range entries {
		parts := strings.Split(entry, "~")
		if len(parts) < 2 {
			continue
		}
		// parts[0]=market(us), parts[1]=code(aapl.oq), parts[2]=name
		code := parts[1]
		// Extract symbol before the dot: "aapl.oq" -> "AAPL"
		if dotIdx := strings.Index(code, "."); dotIdx > 0 {
			code = code[:dotIdx]
		}
		symbols = append(symbols, strings.ToUpper(code))
	}

	if len(symbols) == 0 {
		return []StockResponse{}, nil
	}
	// Limit to top 10 results
	if len(symbols) > 10 {
		symbols = symbols[:10]
	}

	return h.fetchUSStocksBySymbol(ctx, strings.Join(symbols, ","))
}

func (h *StockHandler) fetchUSStocksBySymbol(ctx context.Context, symbolsStr string) ([]StockResponse, error) {
	symbols := strings.Split(symbolsStr, ",")
	// Build Tencent Finance batch query codes: "usAAPL,usNVDA,usTSLA"
	var tCodes []string
	for _, sym := range symbols {
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		tCodes = append(tCodes, "us"+strings.ToUpper(sym))
	}
	if len(tCodes) == 0 {
		return nil, nil
	}

	apiURL := fmt.Sprintf("https://qt.gtimg.cn/q=%s", strings.Join(tCodes, ","))
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// Tencent Finance returns GBK encoding
	utf8Body, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), body)
	if err != nil {
		utf8Body = body
	}
	bodyStr := string(utf8Body)

	var stocks []StockResponse
	for _, sym := range symbols {
		sym = strings.TrimSpace(strings.ToUpper(sym))
		if sym == "" {
			continue
		}
		tCode := "us" + sym
		// Find the line for this symbol: v_usAAPL="200~苹果~AAPL.OQ~247.99~..."
		prefix := fmt.Sprintf("v_%s=\"", tCode)
		start := strings.Index(bodyStr, prefix)
		if start == -1 {
			continue
		}
		start += len(prefix)
		end := strings.Index(bodyStr[start:], "\"")
		if end == -1 {
			continue
		}
		fields := strings.Split(bodyStr[start:start+end], "~")
		// Tencent US stock quote fields:
		// [0]=status(200) [1]=中文名 [2]=code(AAPL.OQ) [3]=currentPrice [4]=previousClose
		// [5]=open [6]=volume [31]=change [32]=changePercent [33]=high [34]=low
		// [44]=English name [45]=xxx [46]=52wHigh [47]=52wLow
		if len(fields) < 45 {
			continue
		}

		currentPrice := parseFloat(fields[3])
		previousClose := parseFloat(fields[4])
		openPrice := parseFloat(fields[5])
		volume := parseFloat(fields[6])
		change := parseFloat(fields[31])
		changePct := parseFloat(fields[32])
		high := parseFloat(fields[33])
		low := parseFloat(fields[34])

		displayName := fields[1] // Chinese name
		engName := ""
		if len(fields) > 44 {
			engName = fields[44]
		}
		if displayName == "" {
			displayName = engName
		}
		if displayName == "" {
			displayName = sym
		}

		stocks = append(stocks, StockResponse{
			ID:            sym,
			Symbol:        sym,
			Name:          displayName,
			Market:        "us_stock",
			CurrentPrice:  currentPrice,
			Change:        change,
			ChangePercent: changePct,
			Volume:        volume / 1e8, // to 亿股
			High:          high,
			Low:           low,
			Open:          openPrice,
			PreviousClose: previousClose,
		})
	}
	return stocks, nil
}

// ── Crypto Search (Binance API) ──────────────────────────────────────────────

// cryptoSymbolMap maps common lowercase tickers to their Binance symbol and display name.
var cryptoSymbolMap = map[string]struct {
	BinanceSymbol string
	DisplayName   string
}{
	"btc": {"BTCUSDT", "Bitcoin"}, "bitcoin": {"BTCUSDT", "Bitcoin"},
	"eth": {"ETHUSDT", "Ethereum"}, "ethereum": {"ETHUSDT", "Ethereum"},
	"bnb": {"BNBUSDT", "BNB"},
	"sol": {"SOLUSDT", "Solana"}, "solana": {"SOLUSDT", "Solana"},
	"xrp": {"XRPUSDT", "XRP"}, "ripple": {"XRPUSDT", "XRP"},
	"ada": {"ADAUSDT", "Cardano"}, "cardano": {"ADAUSDT", "Cardano"},
	"doge": {"DOGEUSDT", "Dogecoin"}, "dogecoin": {"DOGEUSDT", "Dogecoin"},
	"avax": {"AVAXUSDT", "Avalanche"},
	"dot": {"DOTUSDT", "Polkadot"}, "polkadot": {"DOTUSDT", "Polkadot"},
	"link": {"LINKUSDT", "Chainlink"}, "chainlink": {"LINKUSDT", "Chainlink"},
	"ltc": {"LTCUSDT", "Litecoin"}, "litecoin": {"LTCUSDT", "Litecoin"},
	"atom": {"ATOMUSDT", "Cosmos"}, "cosmos": {"ATOMUSDT", "Cosmos"},
	"uni": {"UNIUSDT", "Uniswap"},
	"matic": {"MATICUSDT", "Polygon"},
	"near": {"NEARUSDT", "NEAR"},
	"trx": {"TRXUSDT", "TRON"},
	"etc": {"ETCUSDT", "Ethereum Classic"},
	"bch": {"BCHUSDT", "Bitcoin Cash"},
	"sui": {"SUIUSDT", "Sui"},
	"apt": {"APTUSDT", "Aptos"},
	"arb": {"ARBUSDT", "Arbitrum"},
	"op": {"OPUSDT", "Optimism"},
	"ton": {"TONUSDT", "Toncoin"},
	"pepe": {"PEPEUSDT", "Pepe"},
	"shib": {"SHIBUSDT", "Shiba Inu"},
}

// defaultCryptoSymbols is the list of default crypto symbols shown when no query is provided.
var defaultCryptoSymbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "AVAXUSDT"}

func (h *StockHandler) searchCryptoStocks(ctx context.Context, query string) ([]StockResponse, error) {
	var binanceSymbols []string

	if query == "" {
		binanceSymbols = defaultCryptoSymbols
	} else {
		q := strings.ToLower(strings.TrimSpace(query))
		if info, ok := cryptoSymbolMap[q]; ok {
			binanceSymbols = []string{info.BinanceSymbol}
		} else {
			// Try uppercase symbol + USDT as Binance pair
			binanceSymbols = []string{strings.ToUpper(q) + "USDT"}
		}
	}

	return h.fetchBinanceCryptoStocks(ctx, binanceSymbols)
}

// fetchBinanceCryptoStocks fetches crypto stock data from Binance 24hr ticker.
func (h *StockHandler) fetchBinanceCryptoStocks(ctx context.Context, symbols []string) ([]StockResponse, error) {
	tickers, err := h.fetchBinance24hrTickers(ctx, symbols)
	if err != nil {
		return nil, err
	}

	var stocks []StockResponse
	for _, sym := range symbols {
		t, ok := tickers[sym]
		if !ok {
			continue
		}

		// Extract base symbol: BTCUSDT → BTC
		base := strings.TrimSuffix(sym, "USDT")
		displayName := base
		// Look up display name from map
		if info, ok := cryptoSymbolMap[strings.ToLower(base)]; ok {
			displayName = info.DisplayName
		}

		price := parseFloat(t.LastPrice)
		prevClose := parseFloat(t.PrevClosePrice)
		change := parseFloat(t.PriceChange)
		changePct := parseFloat(t.PriceChangePercent)
		high := parseFloat(t.HighPrice)
		low := parseFloat(t.LowPrice)
		openPrice := parseFloat(t.OpenPrice)
		vol := parseFloat(t.QuoteVolume) // USDT volume

		stocks = append(stocks, StockResponse{
			ID:            base,
			Symbol:        base + "/USDT",
			Name:          displayName,
			Market:        "crypto",
			CurrentPrice:  price,
			Change:        change,
			ChangePercent: changePct,
			Volume:        vol / 1e8, // to 亿
			High:          high,
			Low:           low,
			Open:          openPrice,
			PreviousClose: prevClose,
		})
	}
	return stocks, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Watchlist — CRUD per UserID
// ──────────────────────────────────────────────────────────────────────────────

func (h *StockHandler) GetWatchlist(c *gin.Context) {
	userID := c.GetUint("userID")
	market := c.DefaultQuery("market", "a_share")

	items, err := h.watchlistRepo.GetByUserAndMarket(userID, market)
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get watchlist")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get watchlist"})
		return
	}

	if len(items) == 0 {
		c.JSON(http.StatusOK, []StockResponse{})
		return
	}

	// Fetch real-time prices for all watchlist stocks
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var stocks []StockResponse

	switch market {
	case "a_share":
		var codes []string
		for _, item := range items {
			code := strings.TrimSpace(item.StockCode)
			// If stock_code already has sh/sz prefix (e.g. "sh000001"), use as-is
			if strings.HasPrefix(strings.ToLower(code), "sh") || strings.HasPrefix(strings.ToLower(code), "sz") {
				codes = append(codes, strings.ToLower(code))
			} else {
				prefix := "sh"
				if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
					prefix = "sz"
				}
				codes = append(codes, prefix+code)
			}
		}
		stocks, _ = h.fetchAShareStocksByCode(ctx, strings.Join(codes, ","))
	case "us_stock":
		var symbols []string
		for _, item := range items {
			symbols = append(symbols, item.StockCode)
		}
		stocks, _ = h.fetchUSStocksBySymbol(ctx, strings.Join(symbols, ","))
	case "crypto":
		var binanceSymbols []string
		seen := make(map[string]bool) // Deduplicate symbols (e.g. "BTC" and "btc_price" both map to BTCUSDT)
		for _, item := range items {
			code := strings.TrimSpace(item.StockCode)
			// Map index IDs to real symbols (e.g. "btc_price" → "BTC")
			if mapped, ok := cryptoIndexToSymbol[strings.ToLower(code)]; ok {
				code = mapped
			}
			// Skip non-tradeable indices like "fear_greed"
			if strings.EqualFold(code, "fear_greed") {
				continue
			}
			sym := strings.ToUpper(code)
			// If StockCode is just the base (e.g., "BTC"), append USDT
			if !strings.HasSuffix(sym, "USDT") {
				sym = sym + "USDT"
			}
			if !seen[sym] {
				binanceSymbols = append(binanceSymbols, sym)
				seen[sym] = true
			}
		}
		if len(binanceSymbols) > 0 {
			var fetchErr error
			stocks, fetchErr = h.fetchBinanceCryptoStocks(ctx, binanceSymbols)
			if fetchErr != nil {
				h.logger.WithField("error", fetchErr).Warn("Binance API failed for crypto watchlist")
			}
			// Fallback: if Binance API failed or returned fewer results than expected,
			// fill in missing stocks from DB records so the list is never empty.
			if len(stocks) < len(items) {
				// Build set of IDs already fetched
				fetched := make(map[string]bool, len(stocks))
				for _, s := range stocks {
					fetched[strings.ToUpper(s.ID)] = true
				}
				for _, item := range items {
					base := strings.ToUpper(strings.TrimSpace(item.StockCode))
					if mapped, ok := cryptoIndexToSymbol[strings.ToLower(base)]; ok {
						base = strings.ToUpper(mapped)
					}
					if strings.EqualFold(base, "fear_greed") {
						continue
					}
					if !fetched[base] {
						// Add a placeholder entry from DB so the watchlist is not empty
						stocks = append(stocks, StockResponse{
							ID:     base,
							Symbol: base + "/USDT",
							Name:   item.Name,
							Market: "crypto",
						})
					}
				}
			}
		}
	}

	// Ensure we never return null — always return [] for JSON
	if stocks == nil {
		stocks = []StockResponse{}
	}
	c.JSON(http.StatusOK, stocks)
}

type AddWatchlistRequest struct {
	StockCode string `json:"stock_code" binding:"required"`
	Symbol    string `json:"symbol" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Market    string `json:"market" binding:"required"`
}

func (h *StockHandler) AddToWatchlist(c *gin.Context) {
	userID := c.GetUint("userID")

	var req AddWatchlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.WithField("userID", userID).WithField("market", req.Market).WithField("stock_code", req.StockCode).Info("AddToWatchlist request")

	// Normalize crypto index IDs to real symbols before storing
	// e.g. "btc_price" → "BTC", "eth_price" → "ETH"
	stockCode := req.StockCode
	symbol := req.Symbol
	if req.Market == "crypto" {
		if mapped, ok := cryptoIndexToSymbol[strings.ToLower(stockCode)]; ok {
			stockCode = mapped
			symbol = mapped + "/USDT"
		}
	}

	item := &model.WatchlistItem{
		UserID:    userID,
		Market:    req.Market,
		StockCode: stockCode,
		Symbol:    symbol,
		Name:      req.Name,
	}

	if err := h.watchlistRepo.Add(item); err != nil {
		h.logger.WithField("error", err).Error("Failed to add to watchlist")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add to watchlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Added to watchlist"})
}

type RemoveWatchlistRequest struct {
	StockCode string `json:"stock_code" binding:"required"`
	Market    string `json:"market" binding:"required"`
}

func (h *StockHandler) RemoveFromWatchlist(c *gin.Context) {
	userID := c.GetUint("userID")

	var req RemoveWatchlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.watchlistRepo.Remove(userID, req.Market, req.StockCode); err != nil {
		h.logger.WithField("error", err).Error("Failed to remove from watchlist")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove from watchlist"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Removed from watchlist"})
}

// ──────────────────────────────────────────────────────────────────────────────
// Stock Quote (single) — GET /api/v1/stocks/quote?code=600519&market=a_share
// ──────────────────────────────────────────────────────────────────────────────

func (h *StockHandler) GetStockQuote(c *gin.Context) {
	code := c.Query("code")
	market := c.DefaultQuery("market", "a_share")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// ── Try cache first ──────────────────────────────────────────────────
	if h.cache != nil {
		cacheKey := cache.StockQuoteKey(market, code)
		var cached StockResponse
		if hit, _ := h.cache.GetJSON(c.Request.Context(), cacheKey, &cached); hit && cached.ID != "" {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// ── Cache miss — real-time fetch ─────────────────────────────────────
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	var stocks []StockResponse
	var err error

	switch market {
	case "a_share":
		var fullCode string
		if strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") {
			fullCode = code // Already has prefix
		} else {
			prefix := "sh"
			if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
				prefix = "sz"
			}
			fullCode = prefix + code
		}
		stocks, err = h.fetchAShareStocksByCode(ctx, fullCode)
	case "us_stock":
		stocks, err = h.fetchUSStocksBySymbol(ctx, code)
	case "crypto":
		coinCode := strings.TrimSpace(code)
		// Map index IDs to real symbols (e.g. "btc_price" → "BTC")
		if mapped, ok := cryptoIndexToSymbol[strings.ToLower(coinCode)]; ok {
			coinCode = mapped
		}
		sym := strings.ToUpper(coinCode)
		if !strings.HasSuffix(sym, "USDT") {
			sym = sym + "USDT"
		}
		stocks, err = h.fetchBinanceCryptoStocks(ctx, []string{sym})
	}

	if err != nil || len(stocks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "stock not found"})
		return
	}

	// Write to cache (short TTL — individual stock quote)
	if h.cache != nil {
		_ = h.cache.SetJSON(c.Request.Context(), cache.StockQuoteKey(market, code), stocks[0], 2*time.Second)
	}

	c.JSON(http.StatusOK, stocks[0])
}

// ──────────────────────────────────────────────────────────────────────────────
// K-Line Data — GET /api/v1/stocks/kline?code=600519&market=a_share&days=60
// ──────────────────────────────────────────────────────────────────────────────

type KLineResponse struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Volume float64 `json:"volume"`
}

func (h *StockHandler) GetKLineData(c *gin.Context) {
	code := c.Query("code")
	market := c.DefaultQuery("market", "a_share")
	days := c.DefaultQuery("days", "60")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var klines []KLineResponse
	var err error

	switch market {
	case "a_share":
		klines, err = h.fetchAShareKLine(ctx, code, days)
	case "us_stock":
		klines, err = h.fetchUSStockKLine(ctx, code, days)
	case "crypto":
		klines, err = h.fetchCryptoKLine(ctx, code, days)
	}

	if err != nil {
		h.logger.WithField("code", code).WithField("market", market).WithField("error", err).Warn("Failed to fetch K-line data")
	}
	// Ensure we never return null — always return [] for JSON
	if klines == nil {
		klines = []KLineResponse{}
	}

	c.JSON(http.StatusOK, klines)
}

func (h *StockHandler) fetchAShareKLine(ctx context.Context, code string, days string) ([]KLineResponse, error) {
	// Use Sina Finance K-line API (more reliable than Eastmoney)
	// If code already has sh/sz prefix, use it directly; otherwise infer
	var prefix, pureCode string
	if strings.HasPrefix(code, "sh") || strings.HasPrefix(code, "sz") {
		prefix = code[:2]
		pureCode = code[2:]
	} else {
		pureCode = code
		// Default: 6/9 → SH, 0/3 → SZ
		prefix = "sh"
		if strings.HasPrefix(pureCode, "0") || strings.HasPrefix(pureCode, "3") {
			prefix = "sz"
		}
	}

	klineCount := 60
	if d := parseFloat(days); d > 0 {
		klineCount = int(d)
	}

	symbol := prefix + pureCode
	apiURL := fmt.Sprintf(
		"https://quotes.sina.cn/cn/api/jsonp_v2.php/var%%20_%s=/CN_MarketDataService.getKLineData?symbol=%s&scale=240&ma=no&datalen=%d",
		symbol, symbol, klineCount,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://finance.sina.com.cn")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Response is JSONP: var _shXXXXXX=([{...}, ...]);
	// Extract the JSON array from inside the parentheses
	bodyStr := string(body)
	start := strings.Index(bodyStr, "(")
	end := strings.LastIndex(bodyStr, ")")
	if start == -1 || end <= start {
		return nil, fmt.Errorf("invalid JSONP response for %s", symbol)
	}
	jsonStr := bodyStr[start+1 : end]

	var items []struct {
		Day    string `json:"day"`
		Open   string `json:"open"`
		High   string `json:"high"`
		Low    string `json:"low"`
		Close  string `json:"close"`
		Volume string `json:"volume"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, fmt.Errorf("failed to parse Sina kline JSON: %w", err)
	}

	var klines []KLineResponse
	for _, item := range items {
		klines = append(klines, KLineResponse{
			Date:   item.Day,
			Open:   parseFloat(item.Open),
			Close:  parseFloat(item.Close),
			High:   parseFloat(item.High),
			Low:    parseFloat(item.Low),
			Volume: parseFloat(item.Volume),
		})
	}
	return klines, nil
}

// usIndexTencentCode maps index IDs used by the iOS app to Tencent Finance kline codes.
var usIndexTencentCode = map[string]string{
	"DJI":  "us.DJI",
	"GSPC": "us.INX",
	"IXIC": "us.IXIC",
}

func (h *StockHandler) fetchUSStockKLine(ctx context.Context, symbol string, days string) ([]KLineResponse, error) {
	daysInt := 60
	if d := parseFloat(days); d > 0 {
		daysInt = int(d)
	}

	// Check if the symbol is a US index — use Tencent Finance kline API for indices
	// (Sina API returns null for US indices like DJI/INX/IXIC)
	sym := strings.TrimSpace(symbol)
	if tencentCode, ok := usIndexTencentCode[strings.ToUpper(sym)]; ok {
		return h.fetchTencentKLine(ctx, tencentCode, daysInt)
	}

	// For individual stocks, use Sina Finance US stock K-line API
	lowerSym := strings.ToLower(sym)
	apiURL := fmt.Sprintf(
		"https://stock.finance.sina.com.cn/usstock/api/jsonp_v2.php/var%%20_gb_%s=/US_MinKService.getDailyK?symbol=%s&type=daily&num=%d",
		lowerSym, lowerSym, daysInt,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://finance.sina.com.cn")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Response is JSONP: var _gb_aapl=([{...}, ...]);
	// Sometimes prefixed with: /*<script>location.href='//sina.com';</script>*/
	bodyStr := string(body)
	start := strings.Index(bodyStr, "(")
	end := strings.LastIndex(bodyStr, ")")
	if start == -1 || end <= start {
		return nil, fmt.Errorf("invalid JSONP response for US stock %s", symbol)
	}
	jsonStr := bodyStr[start+1 : end]

	var items []struct {
		Day    string `json:"d"`
		Open   string `json:"o"`
		High   string `json:"h"`
		Low    string `json:"l"`
		Close  string `json:"c"`
		Volume string `json:"v"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, fmt.Errorf("failed to parse Sina US kline JSON: %w", err)
	}

	// Only take the last N days
	if len(items) > daysInt {
		items = items[len(items)-daysInt:]
	}

	var klines []KLineResponse
	for _, item := range items {
		klines = append(klines, KLineResponse{
			Date:   item.Day,
			Open:   parseFloat(item.Open),
			Close:  parseFloat(item.Close),
			High:   parseFloat(item.High),
			Low:    parseFloat(item.Low),
			Volume: parseFloat(item.Volume),
		})
	}
	return klines, nil
}

// fetchTencentKLine fetches K-line data from Tencent Finance web API.
// Works for US indices (us.DJI, us.INX, us.IXIC) and individual US stocks.
func (h *StockHandler) fetchTencentKLine(ctx context.Context, tencentCode string, days int) ([]KLineResponse, error) {
	apiURL := fmt.Sprintf(
		"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?_var=kline_dayqfq&param=%s,day,,,%d,qfq",
		tencentCode, days,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Response format: kline_dayqfq={"code":0,"msg":"","data":{"us.DJI":{"day":[["2026-03-16","46707.400","46946.410","47176.140","46707.400","514985340.000"], ...]}}}
	// Extract JSON from JSONP wrapper
	bodyStr := string(body)
	eqIdx := strings.Index(bodyStr, "=")
	if eqIdx == -1 {
		return nil, fmt.Errorf("invalid Tencent kline response for %s", tencentCode)
	}
	jsonStr := strings.TrimSpace(bodyStr[eqIdx+1:])

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data map[string]struct {
			Day [][]interface{} `json:"day"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse Tencent kline JSON for %s: %w", tencentCode, err)
	}

	stockData, ok := result.Data[tencentCode]
	if !ok || len(stockData.Day) == 0 {
		return nil, fmt.Errorf("no kline data for %s", tencentCode)
	}

	var klines []KLineResponse
	for _, row := range stockData.Day {
		if len(row) < 6 {
			continue
		}
		date, _ := row[0].(string)
		openStr, _ := row[1].(string)
		closeStr, _ := row[2].(string)
		highStr, _ := row[3].(string)
		lowStr, _ := row[4].(string)
		volStr, _ := row[5].(string)

		klines = append(klines, KLineResponse{
			Date:   date,
			Open:   parseFloat(openStr),
			Close:  parseFloat(closeStr),
			High:   parseFloat(highStr),
			Low:    parseFloat(lowStr),
			Volume: parseFloat(volStr),
		})
	}
	return klines, nil
}

// cryptoIndexToSymbol maps special index IDs from fetchCryptoIndices to real Binance symbols.
var cryptoIndexToSymbol = map[string]string{
	"btc_price": "BTC",
	"eth_price": "ETH",
}

func (h *StockHandler) fetchCryptoKLine(ctx context.Context, coinID string, days string) ([]KLineResponse, error) {
	rawID := strings.TrimSpace(coinID)

	// Handle special IDs: fear_greed index has no K-line data
	if strings.EqualFold(rawID, "fear_greed") {
		return []KLineResponse{}, nil
	}

	// Map index IDs to real coin symbols (btc_price → BTC, eth_price → ETH)
	if mapped, ok := cryptoIndexToSymbol[strings.ToLower(rawID)]; ok {
		rawID = mapped
	}

	// Convert coinID to Binance symbol: "btc" or "BTC" → "BTCUSDT"
	sym := strings.ToUpper(rawID)
	if !strings.HasSuffix(sym, "USDT") {
		sym = sym + "USDT"
	}

	limit := 60
	if d := parseFloat(days); d > 0 {
		limit = int(d)
	}
	if limit > 1000 {
		limit = 1000 // Binance API max
	}

	apiURL := fmt.Sprintf(
		"https://data-api.binance.vision/api/v3/klines?symbol=%s&interval=1d&limit=%d",
		sym, limit,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Binance kline response: [[openTime, open, high, low, close, volume, closeTime, ...], ...]
	var rawKlines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawKlines); err != nil {
		return nil, fmt.Errorf("failed to decode Binance kline: %w", err)
	}

	var klines []KLineResponse
	for _, k := range rawKlines {
		if len(k) < 6 {
			continue
		}
		openTime := int64(k[0].(float64))
		t := time.Unix(openTime/1000, 0)

		open, _ := k[1].(string)
		high, _ := k[2].(string)
		low, _ := k[3].(string)
		closeP, _ := k[4].(string)
		vol, _ := k[5].(string)

		klines = append(klines, KLineResponse{
			Date:   t.Format("2006-01-02"),
			Open:   parseFloat(open),
			Close:  parseFloat(closeP),
			High:   parseFloat(high),
			Low:    parseFloat(low),
			Volume: parseFloat(vol),
		})
	}
	return klines, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// News — GET /api/v1/stocks/news?code=600519&market=a_share
// ──────────────────────────────────────────────────────────────────────────────

type NewsResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	Time      string `json:"time"`
	Summary   string `json:"summary"`
	Sentiment string `json:"sentiment"` // positive, negative, neutral
	URL       string `json:"url"`
}

func (h *StockHandler) GetStockNews(c *gin.Context) {
	code := c.Query("code")
	name := c.Query("name")
	market := c.DefaultQuery("market", "a_share")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	var news []NewsResponse
	var err error

	switch market {
	case "a_share":
		news, err = h.fetchAShareNews(ctx, code, name)
	case "us_stock":
		news, err = h.fetchUSStockNews(ctx, code, name)
	case "crypto":
		news, err = h.fetchCryptoNews(ctx, code, name)
	}

	if err != nil {
		h.logger.WithField("error", err).Warn("Failed to fetch news")
		news = []NewsResponse{}
	}

	c.JSON(http.StatusOK, news)
}

func (h *StockHandler) fetchAShareNews(ctx context.Context, code string, name string) ([]NewsResponse, error) {
	// Eastmoney stock news API
	prefix := "1"
	if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
		prefix = "0"
	}

	apiURL := fmt.Sprintf(
		"https://search-api-web.eastmoney.com/search/jsonp?cb=&param={\"uid\":\"\",\"keyword\":\"%s\",\"type\":[\"cmsArticleWebOld\"],\"client\":\"web\",\"clientType\":\"web\",\"clientVersion\":\"curr\",\"param\":{\"cmsArticleWebOld\":{\"searchScope\":\"default\",\"sort\":\"default\",\"pageIndex\":1,\"pageSize\":8,\"preTag\":\"<em>\",\"postTag\":\"</em>\"}}}",
		url.QueryEscape(name),
	)
	_ = prefix

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.eastmoney.com")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// JSONP callback strip
	if idx := strings.Index(bodyStr, "("); idx >= 0 {
		bodyStr = bodyStr[idx+1:]
	}
	if idx := strings.LastIndex(bodyStr, ")"); idx >= 0 {
		bodyStr = bodyStr[:idx]
	}

	var result struct {
		Result struct {
			CmsArticleWebOld []struct {
				Title   string `json:"title"`
				Content string `json:"content"`
				Date    string `json:"date"`
				MediaName string `json:"mediaName"`
				ArticleUrl string `json:"url"`
			} `json:"cmsArticleWebOld"`
		} `json:"result"`
	}

	var news []NewsResponse
	if err := json.Unmarshal([]byte(bodyStr), &result); err != nil {
		// Fallback: use a simpler API or return empty
		return news, nil
	}

	for i, article := range result.Result.CmsArticleWebOld {
		if i >= 8 {
			break
		}
		title := strings.ReplaceAll(article.Title, "<em>", "")
		title = strings.ReplaceAll(title, "</em>", "")

		summary := article.Content
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		summary = strings.ReplaceAll(summary, "<em>", "")
		summary = strings.ReplaceAll(summary, "</em>", "")

		sentiment := "neutral"
		news = append(news, NewsResponse{
			ID:        fmt.Sprintf("%s_news_%d", code, i),
			Title:     title,
			Source:    article.MediaName,
			Time:      article.Date,
			Summary:   summary,
			Sentiment: sentiment,
			URL:       article.ArticleUrl,
		})
	}
	return news, nil
}

func (h *StockHandler) fetchUSStockNews(ctx context.Context, symbol string, name string) ([]NewsResponse, error) {
	// Use Eastmoney search API (accessible from mainland China, same as A-share news)
	query := symbol
	if name != "" {
		query = name
	}
	apiURL := fmt.Sprintf(
		"https://search-api-web.eastmoney.com/search/jsonp?cb=&param={\"uid\":\"\",\"keyword\":\"%s\",\"type\":[\"cmsArticleWebOld\"],\"client\":\"web\",\"clientType\":\"web\",\"clientVersion\":\"curr\",\"param\":{\"cmsArticleWebOld\":{\"searchScope\":\"default\",\"sort\":\"default\",\"pageIndex\":1,\"pageSize\":8,\"preTag\":\"<em>\",\"postTag\":\"</em>\"}}}",
		url.QueryEscape(query),
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.eastmoney.com")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// JSONP callback strip
	if idx := strings.Index(bodyStr, "("); idx >= 0 {
		bodyStr = bodyStr[idx+1:]
	}
	if idx := strings.LastIndex(bodyStr, ")"); idx >= 0 {
		bodyStr = bodyStr[:idx]
	}

	var result struct {
		Result struct {
			CmsArticleWebOld []struct {
				Title      string `json:"title"`
				Content    string `json:"content"`
				Date       string `json:"date"`
				MediaName  string `json:"mediaName"`
				ArticleUrl string `json:"url"`
			} `json:"cmsArticleWebOld"`
		} `json:"result"`
	}

	var news []NewsResponse
	if err := json.Unmarshal([]byte(bodyStr), &result); err != nil {
		return news, nil
	}

	for i, article := range result.Result.CmsArticleWebOld {
		if i >= 8 {
			break
		}
		title := strings.ReplaceAll(article.Title, "<em>", "")
		title = strings.ReplaceAll(title, "</em>", "")

		summary := article.Content
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		summary = strings.ReplaceAll(summary, "<em>", "")
		summary = strings.ReplaceAll(summary, "</em>", "")

		sentiment := "neutral"
		news = append(news, NewsResponse{
			ID:        fmt.Sprintf("%s_news_%d", symbol, i),
			Title:     title,
			Source:    article.MediaName,
			Time:      article.Date,
			Summary:   summary,
			Sentiment: sentiment,
			URL:       article.ArticleUrl,
		})
	}
	return news, nil
}

func (h *StockHandler) fetchCryptoNews(ctx context.Context, coinID string, name string) ([]NewsResponse, error) {
	// No free crypto news API available; return empty placeholder
	return []NewsResponse{}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

func toFloatVal(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case nil:
		return 0, false
	}
	return 0, false
}
