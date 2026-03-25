package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/search"
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
	searcher      search.Searcher      // Web search for news (may be NoopSearcher)
	llmClient     *llm.OpenAIClient    // LLM client for AI summaries (may be nil)
	fmpAPIKey     string               // FMP (Financial Modeling Prep) API key
}

// NewStockHandler creates a new StockHandler.
// If quoteCache is non-nil, background tickers will be started to pre-populate
// Redis with fresh market data; HTTP handlers will read from the cache first.
func NewStockHandler(watchlistRepo *repository.WatchlistRepository, logger *logger.Logger, quoteCache *cache.QuoteCache, searcher search.Searcher, llmClient *llm.OpenAIClient) *StockHandler {
	h := &StockHandler{
		watchlistRepo: watchlistRepo,
		logger:        logger,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		cache:         quoteCache,
		searcher:      searcher,
		llmClient:     llmClient,
		fmpAPIKey:     os.Getenv("FMP_API_KEY"),
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

// fetchMinuteData fetches minute-level price data for sparkline charts.
// Primary: Eastmoney trends2 API. Fallback: Sina 5-minute K-line API.
func (h *StockHandler) fetchTencentMinuteData(ctx context.Context, code string) ([]float64, error) {
	// Try Eastmoney first
	prices, err := h.fetchEastmoneySparkline(ctx, code)
	if err == nil && len(prices) > 0 {
		return h.downsampleSparkline(prices), nil
	}

	// Fallback: Sina 5-minute K-line
	prices, err = h.fetchSinaSparkline(ctx, code)
	if err == nil && len(prices) > 0 {
		return h.downsampleSparkline(prices), nil
	}
	return nil, fmt.Errorf("all sparkline sources failed for %s", code)
}

func (h *StockHandler) downsampleSparkline(prices []float64) []float64 {
	if len(prices) > 20 {
		step := len(prices) / 20
		var sampled []float64
		for i := 0; i < len(prices); i += step {
			sampled = append(sampled, prices[i])
		}
		return sampled
	}
	return prices
}

func (h *StockHandler) fetchEastmoneySparkline(ctx context.Context, code string) ([]float64, error) {
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
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			p := parseFloat(parts[2])
			if p > 0 {
				prices = append(prices, p)
			}
		}
	}
	return prices, nil
}

// fetchSinaSparkline fetches recent 5-minute K-line close prices from Sina Finance.
func (h *StockHandler) fetchSinaSparkline(ctx context.Context, code string) ([]float64, error) {
	apiURL := fmt.Sprintf(
		"https://quotes.sina.cn/cn/api/jsonp_v2.php/var%%20_%s_spark=/CN_MarketDataService.getKLineData?symbol=%s&scale=5&ma=no&datalen=48",
		code, code,
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

	bodyStr := string(body)
	start := strings.Index(bodyStr, "(")
	end := strings.LastIndex(bodyStr, ")")
	if start == -1 || end <= start {
		return nil, fmt.Errorf("invalid Sina sparkline JSONP")
	}

	var items []struct {
		Close string `json:"close"`
	}
	if err := json.Unmarshal([]byte(bodyStr[start+1:end]), &items); err != nil {
		return nil, err
	}

	var prices []float64
	for _, item := range items {
		p := parseFloat(item.Close)
		if p > 0 {
			prices = append(prices, p)
		}
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

		// Extract exchange prefix from the line header: v_sh000001="..." or v_sz000001="..."
		linePrefix := ""
		if strings.Contains(line, "v_sh") {
			linePrefix = "SH"
		} else if strings.Contains(line, "v_sz") {
			linePrefix = "SZ"
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

		// Determine symbol prefix: prefer the prefix from the line header (v_shXXXXXX / v_szXXXXXX)
		// which is authoritative, fall back to guessing from code digits only if header is missing
		prefix := linePrefix
		if prefix == "" {
			prefix = "SH"
			if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
				prefix = "SZ"
			}
		}

		stocks = append(stocks, StockResponse{
			ID:            strings.ToLower(prefix) + code,
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
				// Prefer extracting prefix from the stored Symbol field (e.g. "SH000001" → "sh")
				// This avoids ambiguity for codes like 000001 which is both SH (上证指数) and SZ (平安银行)
				prefix := ""
				sym := strings.ToLower(strings.TrimSpace(item.Symbol))
				if strings.HasPrefix(sym, "sh") {
					prefix = "sh"
				} else if strings.HasPrefix(sym, "sz") {
					prefix = "sz"
				} else {
					// Fallback: guess from code digits
					prefix = "sh"
					if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
						prefix = "sz"
					}
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

	// For A-shares: clean up any old-format record (without prefix) to avoid duplicates
	if req.Market == "a_share" {
		lc := strings.ToLower(stockCode)
		if strings.HasPrefix(lc, "sh") || strings.HasPrefix(lc, "sz") {
			bareCode := lc[2:]
			_ = h.watchlistRepo.Remove(userID, req.Market, bareCode)
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

	code := req.StockCode

	// Try removing with the exact code first
	if err := h.watchlistRepo.Remove(userID, req.Market, code); err != nil {
		h.logger.WithField("error", err).Error("Failed to remove from watchlist")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove from watchlist"})
		return
	}

	// Also try the other format for backward compatibility:
	// If code has sh/sz prefix (new format), also try without prefix (old format)
	// If code has no prefix (old format), also try with prefix (new format)
	if req.Market == "a_share" {
		lc := strings.ToLower(code)
		if strings.HasPrefix(lc, "sh") || strings.HasPrefix(lc, "sz") {
			bareCode := lc[2:]
			_ = h.watchlistRepo.Remove(userID, req.Market, bareCode)
		} else if len(code) == 6 {
			prefix := "sh"
			if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
				prefix = "sz"
			}
			_ = h.watchlistRepo.Remove(userID, req.Market, prefix+code)
		}
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
// K-Line Data — GET /api/v1/stocks/kline?code=600519&market=a_share&period=1d&limit=60
// Supported periods: 5m, 15m, 30m, 1h, 4h, 1d, 1w
// ──────────────────────────────────────────────────────────────────────────────

type KLineResponse struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Volume float64 `json:"volume"`
}

// sinaScaleMap maps our unified period strings to Sina Finance API scale values (minutes).
// Note: Sina's scale parameter represents the time window in minutes for each candle.
// For daily K-line (1d), we still use scale=240 but with special handling in fetchAShareKLine
// to ensure we get proper daily data, not 4-hour intraday data.
var sinaScaleMap = map[string]string{
	"5m":  "5",
	"15m": "15",
	"30m": "30",
	"1h":  "60",
	"4h":  "240",
	"1d":  "240", // Daily uses same scale but different datalen and aggregation logic
	"1w":  "1680",
}

// binanceIntervalMap maps our unified period strings to Binance API interval values.
var binanceIntervalMap = map[string]string{
	"5m":  "5m",
	"15m": "15m",
	"30m": "30m",
	"1h":  "1h",
	"4h":  "4h",
	"1d":  "1d",
	"1w":  "1w",
}

// defaultLimitForPeriod returns a sensible default number of candles for each period.
// Initial load provides 120-180 candles for a good visual experience.
func defaultLimitForPeriod(period string) int {
	switch period {
	case "5m":
		return 120 // ~2.5 trading days
	case "15m":
		return 160 // ~10 trading days
	case "30m":
		return 120 // ~7.5 trading days
	case "1h":
		return 120 // ~5 weeks
	case "4h":
		return 120 // ~2.5 months
	case "1d":
		return 150 // ~7.5 months
	case "1w":
		return 120 // ~2.3 years
	default:
		return 150
	}
}

func (h *StockHandler) GetKLineData(c *gin.Context) {
	code := c.Query("code")
	market := c.DefaultQuery("market", "a_share")
	period := c.DefaultQuery("period", "1d")

	// Backward compatibility: if old "days" param is supplied, use it for daily data
	if c.Query("period") == "" && c.Query("days") != "" {
		period = "1d"
	}

	// Validate period
	validPeriods := map[string]bool{"5m": true, "15m": true, "30m": true, "1h": true, "4h": true, "1d": true, "1w": true}
	if !validPeriods[period] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period, supported: 5m, 15m, 30m, 1h, 4h, 1d, 1w"})
		return
	}

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// Determine limit (number of candles)
	limit := defaultLimitForPeriod(period)
	if c.Query("limit") != "" {
		if d := parseFloat(c.Query("limit")); d > 0 {
			limit = int(d)
		}
	} else if c.Query("days") != "" {
		if d := parseFloat(c.Query("days")); d > 0 {
			limit = int(d)
		}
	}

	// Overall timeout for the entire request (including retries)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// Exponential backoff retry: up to 3 retries with delays of 500ms, 1s, 2s
	const maxRetries = 3
	var klines []KLineResponse
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Wait before retrying (exponential backoff: 500ms * 2^attempt)
		if attempt > 0 {
			backoff := time.Duration(float64(500*time.Millisecond) * math.Pow(2, float64(attempt-1)))
			h.logger.WithField("code", code).WithField("market", market).WithField("period", period).
				WithField("attempt", attempt).WithField("backoff_ms", backoff.Milliseconds()).
				Info("Retrying K-line fetch")

			select {
			case <-ctx.Done():
				// Overall timeout expired — stop retrying
				lastErr = fmt.Errorf("获取数据超时")
				goto respond
			case <-time.After(backoff):
				// Continue with retry
			}
		}

		// Create a per-attempt timeout context (5s per attempt)
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 5*time.Second)

		var err error
		switch market {
		case "a_share":
			klines, err = h.fetchAShareKLine(attemptCtx, code, period, limit)
		case "us_stock":
			klines, err = h.fetchUSStockKLine(attemptCtx, code, period, limit)
		case "crypto":
			klines, err = h.fetchCryptoKLine(attemptCtx, code, period, limit)
		}
		attemptCancel()

		if err != nil {
			lastErr = err
			h.logger.WithField("code", code).WithField("market", market).WithField("period", period).
				WithField("attempt", attempt).WithField("error", err).Warn("K-line fetch attempt failed")
			continue
		}

		if len(klines) > 0 {
			// Success — return data
			c.JSON(http.StatusOK, klines)
			return
		}

		// Got empty result but no error — treat as retriable
		lastErr = fmt.Errorf("upstream returned empty data")
	}

respond:
	// All retries exhausted — return error information to the client
	if lastErr != nil {
		h.logger.WithField("code", code).WithField("market", market).WithField("period", period).
			WithField("error", lastErr).Error("K-line fetch failed after all retries")
	}

	// Return a structured error response with HTTP 200 but an error field,
	// so the client can distinguish between "no data" and "error"
	errMsg := "获取数据超时"
	if lastErr != nil && !strings.Contains(lastErr.Error(), "超时") {
		errMsg = "获取K线数据失败，请稍后重试"
	}
	c.JSON(http.StatusOK, gin.H{
		"error":   errMsg,
		"data":    []KLineResponse{},
		"retries": maxRetries,
	})
}

func (h *StockHandler) fetchAShareKLine(ctx context.Context, code string, period string, limit int) ([]KLineResponse, error) {
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

	// Map period to Sina scale (minutes).
	// Sina supports: 5, 15, 30, 60, 240 for intraday; for weekly we use
	// scale=1680 (7 days × 240 min) which Sina treats as weekly.
	scale := "240" // default daily
	if s, ok := sinaScaleMap[period]; ok {
		scale = s
	}

	symbol := prefix + pureCode
	apiURL := fmt.Sprintf(
		"https://quotes.sina.cn/cn/api/jsonp_v2.php/var%%20_%s=/CN_MarketDataService.getKLineData?symbol=%s&scale=%s&ma=no&datalen=%d",
		symbol, symbol, scale, limit,
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

	// For daily K-line: Sina scale=240 doesn't return today's incomplete bar during trading hours.
	// Fetch today's hourly bars (scale=60) and aggregate them into a single daily bar.
	if period == "1d" && len(klines) > 0 {
		todayStr := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
		lastDate := klines[len(klines)-1].Date
		if len(lastDate) >= 10 {
			lastDate = lastDate[:10]
		}
		if lastDate != todayStr {
			todayBar := h.aggregateTodayBar(ctx, symbol, todayStr)
			if todayBar != nil {
				klines = append(klines, *todayBar)
			}
		}
	}

	return klines, nil
}

// aggregateTodayBar fetches today's hourly bars from Sina (scale=60) and aggregates
// them into a single daily K-line bar for the current trading day.
func (h *StockHandler) aggregateTodayBar(ctx context.Context, symbol, todayStr string) *KLineResponse {
	apiURL := fmt.Sprintf(
		"https://quotes.sina.cn/cn/api/jsonp_v2.php/var%%20_%s_60=/CN_MarketDataService.getKLineData?symbol=%s&scale=60&ma=no&datalen=8",
		symbol, symbol,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://finance.sina.com.cn")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	bodyStr := string(body)
	hStart := strings.Index(bodyStr, "(")
	hEnd := strings.LastIndex(bodyStr, ")")
	if hStart == -1 || hEnd <= hStart {
		return nil
	}

	var hourItems []struct {
		Day    string `json:"day"`
		Open   string `json:"open"`
		High   string `json:"high"`
		Low    string `json:"low"`
		Close  string `json:"close"`
		Volume string `json:"volume"`
	}
	if err := json.Unmarshal([]byte(bodyStr[hStart+1:hEnd]), &hourItems); err != nil {
		return nil
	}

	// Filter only bars from today and aggregate into a single daily bar
	var todayOpen, todayClose, todayHigh, todayLow, todayVolume float64
	first := true
	for _, item := range hourItems {
		if len(item.Day) >= 10 && item.Day[:10] == todayStr {
			o := parseFloat(item.Open)
			c := parseFloat(item.Close)
			hi := parseFloat(item.High)
			lo := parseFloat(item.Low)
			v := parseFloat(item.Volume)
			if first {
				todayOpen = o
				todayHigh = hi
				todayLow = lo
				first = false
			} else {
				if hi > todayHigh {
					todayHigh = hi
				}
				if lo < todayLow {
					todayLow = lo
				}
			}
			todayClose = c
			todayVolume += v
		}
	}

	if first {
		return nil // no today data found
	}

	return &KLineResponse{
		Date:   todayStr,
		Open:   todayOpen,
		Close:  todayClose,
		High:   todayHigh,
		Low:    todayLow,
		Volume: todayVolume,
	}
}
var usIndexTencentCode = map[string]string{
	"DJI":  "us.DJI",
	"GSPC": "us.INX",
	"IXIC": "us.IXIC",
}

func (h *StockHandler) fetchUSStockKLine(ctx context.Context, symbol string, period string, limit int) ([]KLineResponse, error) {
	// Check if the symbol is a US index — use Tencent Finance kline API for indices
	// (Sina API returns null for US indices like DJI/INX/IXIC)
	sym := strings.TrimSpace(symbol)
	if tencentCode, ok := usIndexTencentCode[strings.ToUpper(sym)]; ok {
		return h.fetchTencentKLine(ctx, tencentCode, period, limit)
	}

	// For individual US stocks: Sina only provides daily data.
	// For sub-daily periods, we fall back to daily data.
	lowerSym := strings.ToLower(sym)
	apiURL := fmt.Sprintf(
		"https://stock.finance.sina.com.cn/usstock/api/jsonp_v2.php/var%%20_gb_%s=/US_MinKService.getDailyK?symbol=%s&type=daily&num=%d",
		lowerSym, lowerSym, limit,
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

	// Only take the last N items
	if len(items) > limit {
		items = items[len(items)-limit:]
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
// Tencent API supports "day" (daily) and "week" (weekly) periods.
func (h *StockHandler) fetchTencentKLine(ctx context.Context, tencentCode string, period string, limit int) ([]KLineResponse, error) {
	// Tencent only supports day / week; for sub-daily periods fall back to day
	tqPeriod := "day"
	if period == "1w" {
		tqPeriod = "week"
	}
	apiURL := fmt.Sprintf(
		"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?_var=kline_dayqfq&param=%s,%s,,,%d,qfq",
		tencentCode, tqPeriod, limit,
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

// cryptoIndexNameMap maps Chinese index names to the canonical English display names.
// This ensures that market-overview (大盘) and watchlist (自选) use the same code/name
// for the same asset, producing identical search queries and sharing the same cache key.
var cryptoIndexNameMap = map[string]string{
	"比特币":  "Bitcoin",
	"以太坊":  "Ethereum",
	"币安币":  "BNB",
	"索拉纳":  "Solana",
	"瑞波币":  "XRP",
	"卡尔达诺": "Cardano",
	"狗狗币":  "Dogecoin",
	"雪崩":   "Avalanche",
	"波卡":   "Polkadot",
	"链环":   "Chainlink",
	"莱特币":  "Litecoin",
	"宇宙币":  "Cosmos",
	"波场":   "TRON",
	"以太经典": "Ethereum Classic",
}

// normalizeCryptoParams ensures that regardless of whether the request comes from
// the market overview (code="btc_price", name="比特币") or the watchlist
// (code="BTC", name="Bitcoin"), we always use the same canonical code and name.
func normalizeCryptoParams(code, name string) (string, string) {
	// Step 1: Map index IDs like "btc_price" → "BTC"
	if mapped, ok := cryptoIndexToSymbol[strings.ToLower(code)]; ok {
		code = mapped
	}
	// Step 2: Map Chinese names like "比特币" → "Bitcoin"
	if mapped, ok := cryptoIndexNameMap[name]; ok {
		name = mapped
	}
	return code, name
}

func (h *StockHandler) fetchCryptoKLine(ctx context.Context, coinID string, period string, limit int) ([]KLineResponse, error) {
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

	if limit > 1000 {
		limit = 1000 // Binance API max
	}

	// Map our unified period to Binance interval
	interval := "1d"
	if bi, ok := binanceIntervalMap[period]; ok {
		interval = bi
	}

	apiURL := fmt.Sprintf(
		"https://data-api.binance.vision/api/v3/klines?symbol=%s&interval=%s&limit=%d",
		sym, interval, limit,
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

	// Choose date format based on period — include time for intraday
	dateFormat := "2006-01-02"
	if period != "1d" && period != "1w" {
		dateFormat = "2006-01-02 15:04"
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
			Date:   t.Format(dateFormat),
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
	Analysis  string `json:"analysis,omitempty"` // Detailed AI analysis (200-300 chars) for detail page
	Sentiment string `json:"sentiment"`          // positive, negative, neutral
	URL       string `json:"url"`
}

// ── Server-side news cache (5 min TTL) ──────────────────────────────────────

type newsCacheEntry struct {
	data      []NewsResponse
	timestamp time.Time
}

var (
	newsMemCache   = make(map[string]*newsCacheEntry)
	newsMemCacheMu sync.RWMutex
	newsCacheTTL   = 5 * time.Minute
)

func getNewsCacheKey(code, market string) string {
	return market + "_" + code
}

func getNewsFromMemCache(code, market string) ([]NewsResponse, bool) {
	key := getNewsCacheKey(code, market)
	newsMemCacheMu.RLock()
	defer newsMemCacheMu.RUnlock()
	entry, ok := newsMemCache[key]
	if !ok || time.Since(entry.timestamp) > newsCacheTTL {
		return nil, false
	}
	return entry.data, true
}

func setNewsToMemCache(code, market string, data []NewsResponse) {
	key := getNewsCacheKey(code, market)
	newsMemCacheMu.Lock()
	newsMemCache[key] = &newsCacheEntry{data: data, timestamp: time.Now()}
	newsMemCacheMu.Unlock()
}

// cryptoChineseNames maps crypto display names (English) to Chinese names for better
// search relevance on Chinese news sites and for title matching.
var cryptoChineseNames = map[string]string{
	"Bitcoin":          "比特币",
	"BTC":              "比特币",
	"Ethereum":         "以太坊",
	"ETH":              "以太坊",
	"BNB":              "币安币",
	"Solana":           "索拉纳",
	"SOL":              "索拉纳",
	"XRP":              "瑞波币",
	"Cardano":          "卡尔达诺",
	"ADA":              "卡尔达诺",
	"Dogecoin":         "狗狗币",
	"DOGE":             "狗狗币",
	"Avalanche":        "雪崩",
	"AVAX":             "雪崩",
	"Polkadot":         "波卡",
	"DOT":              "波卡",
	"Chainlink":        "链环",
	"LINK":             "链环",
	"Litecoin":         "莱特币",
	"LTC":              "莱特币",
	"Cosmos":           "宇宙币",
	"ATOM":             "宇宙币",
	"Uniswap":          "Uniswap",
	"UNI":              "Uniswap",
	"Polygon":          "Polygon",
	"MATIC":            "Polygon",
	"NEAR":             "NEAR",
	"TRON":             "波场",
	"TRX":              "波场",
	"Ethereum Classic": "以太经典",
	"ETC":              "以太经典",
	"Bitcoin Cash":     "比特现金",
	"BCH":              "比特现金",
	"Sui":              "Sui",
	"Aptos":            "Aptos",
	"APT":              "Aptos",
	"Arbitrum":         "Arbitrum",
	"ARB":              "Arbitrum",
	"Optimism":         "Optimism",
	"OP":               "Optimism",
	"Toncoin":          "TON币",
	"TON":              "TON币",
	"Pepe":             "Pepe",
	"PEPE":             "Pepe",
	"Shiba Inu":        "柴犬币",
	"SHIB":             "柴犬币",
}

// qualityDomains are curated financial news sources with real stock-specific content.
// Search queries are restricted to these domains to avoid homepage links and irrelevant results.
var qualityDomains = map[string][]string{
	"a_share": {
		"finance.sina.com.cn", "stock.eastmoney.com", "xueqiu.com",
		"cls.cn", "wallstreetcn.com", "10jqka.com.cn",
		"stcn.com", "cs.com.cn", "nbd.com.cn", "yicai.com",
		"jin10.com", "cninfo.com.cn", "ssrn.com",
	},
	"us_stock": {
		"xueqiu.com", "wallstreetcn.com", "finance.sina.com.cn",
		"cnbc.com", "bloomberg.com", "reuters.com",
		"wsj.com", "eastmoney.com", "jin10.com", "cls.cn",
		"36kr.com", "yicai.com",
	},
	"crypto": {
		"jinse.cn", "jinse.com", "8btc.com", "theblockbeats.info",
		"wallstreetcn.com", "xueqiu.com", "cls.cn", "36kr.com",
		"odaily.news", "panewslab.com", "techflowpost.com",
		"jin10.com", "foresightnews.pro", "marsbit.co",
	},
}

// normalizeSearchDate converts various date formats from search engines into "2006-01-02".
// Brave returns page_age as ISO 8601 (e.g. "2025-12-17T08:35:00Z") or relative like "3 hours ago".
// Serper returns date like "Dec 17, 2025", "3 days ago", "17 hours ago".
// Falls back to today's date if parsing fails or date is empty.
func normalizeSearchDate(raw string) string {
	if raw == "" {
		return time.Now().Format("2006-01-02")
	}

	raw = strings.TrimSpace(raw)

	// --- Try common absolute date formats ---
	formats := []string{
		time.RFC3339,                // 2025-12-17T08:35:00Z
		"2006-01-02T15:04:05Z0700", // 2025-12-17T08:35:00+0800
		"2006-01-02T15:04:05",      // 2025-12-17T08:35:00
		"2006-01-02",               // 2025-12-17
		"Jan 2, 2006",              // Dec 17, 2025
		"January 2, 2006",          // December 17, 2025
		"Jan 02, 2006",             // Dec 17, 2025
		"2 Jan 2006",               // 17 Dec 2025
		"02 Jan 2006",              // 17 Dec 2025
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}

	// --- Handle relative time expressions ---
	rawLower := strings.ToLower(raw)

	// Chinese relative: "3小时前", "2天前", "1周前"
	// English relative: "3 hours ago", "2 days ago", "1 week ago"
	type relPattern struct {
		keywords []string
		unit     time.Duration
	}
	patterns := []relPattern{
		{[]string{"小时前", "hour"}, time.Hour},
		{[]string{"分钟前", "minute"}, time.Minute},
		{[]string{"天前", "day"}, 24 * time.Hour},
		{[]string{"周前", "week"}, 7 * 24 * time.Hour},
	}

	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(rawLower, kw) {
				// Extract the number
				num := 1
				for _, ch := range raw {
					if ch >= '0' && ch <= '9' {
						num = int(ch - '0')
						break
					}
				}
				// For multi-digit numbers, parse more carefully
				numStr := ""
				for _, ch := range raw {
					if ch >= '0' && ch <= '9' {
						numStr += string(ch)
					} else if numStr != "" {
						break
					}
				}
				if numStr != "" {
					parsed := 0
					for _, ch := range numStr {
						parsed = parsed*10 + int(ch-'0')
					}
					if parsed > 0 {
						num = parsed
					}
				}

				t := time.Now().Add(-time.Duration(num) * p.unit)
				return t.Format("2006-01-02")
			}
		}
	}

	// Fallback: today
	return time.Now().Format("2006-01-02")
}

// isNonArticleURL checks if a URL is a homepage, category page, or stock quote/profile page
// rather than an actual news article. This filters out URLs like:
//   - xueqiu.com/S/SZ300750 (stock profile)
//   - xueqiu.com/k?q=SZ300750 (stock quote)
//   - finance.sina.com.cn/realstock/company/sz300750/nc.shtml (stock page)
//   - stock.eastmoney.com/a/cSZ300750.html (stock page)
//   - 10jqka.com.cn/300750/ (stock page)
func isNonArticleURL(rawURL string, stockCode string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	path := strings.TrimRight(u.Path, "/")
	pathLower := strings.ToLower(path)
	queryStr := strings.ToLower(u.RawQuery)

	// --- 1. Homepage / category page detection ---
	if path == "" || strings.Count(path, "/") <= 1 {
		pathPart := strings.TrimPrefix(path, "/")
		if len(pathPart) < 15 && !strings.ContainsAny(pathPart, "0123456789") {
			return true
		}
	}

	// --- 2. Known stock quote/profile page patterns ---

	// Xueqiu: /S/SZ300750, /S/SH600519, /S/AAPL, /k?q=...
	if strings.Contains(host, "xueqiu.com") {
		if strings.HasPrefix(pathLower, "/s/") || strings.HasPrefix(pathLower, "/k") {
			return true
		}
	}

	// Sina Finance: /realstock/company/sz300750/..., /stock/hkstock/..., /corp/go.php/stockIndustryNews/..., quote pages
	if strings.Contains(host, "sina.com.cn") {
		if strings.Contains(pathLower, "/realstock/") || strings.Contains(pathLower, "/stock/") {
			return true
		}
		// /corp/go.php/... are stock profile / industry aggregate pages, not articles
		if strings.Contains(pathLower, "/corp/") {
			return true
		}
		if strings.Contains(pathLower, "quote") {
			return true
		}
	}

	// Eastmoney stock pages: /a/cSZ300750.html, /stockpage/...
	if strings.Contains(host, "eastmoney.com") {
		if strings.Contains(pathLower, "stockpage") || strings.Contains(pathLower, "quote") {
			return true
		}
		// /a/cSZ300750.html or /a/csh600519.html — these are stock profile, not articles
		// Eastmoney article URLs look like: /a/202403231234567890.html (long numeric ID)
		if strings.HasPrefix(pathLower, "/a/c") && len(path) < 25 {
			return true
		}
	}

	// 10jqka: /300750/ or /stock/... — stock profile pages
	if strings.Contains(host, "10jqka.com.cn") {
		// Pure numeric path = stock page (e.g. /300750/)
		trimmed := strings.Trim(path, "/")
		if len(trimmed) > 0 && len(trimmed) <= 10 {
			allDigit := true
			for _, c := range trimmed {
				if c < '0' || c > '9' {
					allDigit = false
					break
				}
			}
			if allDigit {
				return true
			}
		}
	}

	// --- 3. Generic quote/profile page path patterns (any domain) ---
	if strings.Contains(pathLower, "/quotes/") || strings.Contains(pathLower, "/quote/") ||
		strings.Contains(pathLower, "/quote.") {
		return true
	}
	if stockCode != "" {
		codeLower := strings.ToLower(stockCode)
		// Match code with common prefixes: sz300750, sh600519, SZ.300750, etc.
		codeVariants := []string{
			codeLower,
			"sz" + codeLower, "sh" + codeLower,
			"sz." + codeLower, "sh." + codeLower,
		}
		for _, variant := range codeVariants {
			// Check if the URL path is essentially just the stock code (not part of a longer article slug)
			if pathLower == "/"+variant || pathLower == "/s/"+variant ||
				strings.HasSuffix(pathLower, "/"+variant) {
				return true
			}
		}
		// Also check query parameters for stock code (e.g. ?q=SZ300750)
		for _, variant := range codeVariants {
			if strings.Contains(queryStr, variant) {
				return true
			}
		}
	}

	return false
}

func (h *StockHandler) GetStockNews(c *gin.Context) {
	code := c.Query("code")
	name := c.Query("name")
	market := c.DefaultQuery("market", "a_share")

	// Normalize crypto params so market-overview and watchlist share the same code/name/cache
	if market == "crypto" {
		code, name = normalizeCryptoParams(code, name)
	}

	// Check server-side cache first
	if cached, ok := getNewsFromMemCache(code, market); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 18*time.Second)
	defer cancel()

	// Try search-based news first (multi-round Serper/Brave search)
	news, err := h.fetchNewsViaSearch(ctx, code, name, market)
	if err != nil || len(news) == 0 {
		// Fallback to legacy API for markets that have dedicated sources
		switch market {
		case "a_share":
			news, err = h.fetchAShareNews(ctx, code, name)
		case "us_stock":
			news, err = h.fetchUSStockNews(ctx, code, name)
		}
	}

	if err != nil {
		h.logger.WithField("error", err).Warn("Failed to fetch news")
		news = []NewsResponse{}
	}
	if news == nil {
		news = []NewsResponse{}
	}

	// Cache non-empty results
	if len(news) > 0 {
		setNewsToMemCache(code, market, news)
	}

	c.JSON(http.StatusOK, news)
}

// ── Enhance Single News — GET /api/v1/stocks/news/enhance ───────────────────
// Called when user opens a news detail page. Returns AI summary + analysis
// for a specific news item identified by news_id within a stock's cached news.
// If the cached entry already has AI analysis, returns it immediately.
// Otherwise, calls LLM synchronously for just that one news item.

func (h *StockHandler) EnhanceNewsItem(c *gin.Context) {
	newsID := c.Query("news_id")
	code := c.Query("code")
	market := c.DefaultQuery("market", "a_share")
	name := c.Query("name")

	if newsID == "" || code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "news_id and code are required"})
		return
	}

	// Check if cached news already has AI enhancement for this item
	if cached, ok := getNewsFromMemCache(code, market); ok {
		for _, n := range cached {
			if n.ID == newsID && n.Analysis != "" {
				c.JSON(http.StatusOK, gin.H{
					"summary":   n.Summary,
					"analysis":  n.Analysis,
					"sentiment": n.Sentiment,
				})
				return
			}
		}
		// Found the item but no analysis yet — enhance just this one
		for _, n := range cached {
			if n.ID == newsID {
				enhanced := h.enhanceSingleNews(c.Request.Context(), n, name, code)
				// Update the cached entry
				h.updateCachedNewsItem(code, market, newsID, enhanced)
				c.JSON(http.StatusOK, enhanced)
				return
			}
		}
	}

	// No cache hit — caller should provide title+summary as fallback
	title := c.Query("title")
	summary := c.Query("summary")
	source := c.Query("source")
	if title == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "news item not found in cache and no title provided"})
		return
	}

	fakeNews := NewsResponse{
		ID:      newsID,
		Title:   title,
		Summary: summary,
		Source:  source,
	}
	enhanced := h.enhanceSingleNews(c.Request.Context(), fakeNews, name, code)
	c.JSON(http.StatusOK, enhanced)
}

// enhanceSingleNews calls LLM to generate summary + analysis for a single news item.
// NOTE: Does NOT generate sentiment — sentiment is already set during the initial news fetch.
func (h *StockHandler) enhanceSingleNews(ctx context.Context, news NewsResponse, stockName, stockCode string) gin.H {
	if h.llmClient == nil {
		return gin.H{
			"summary":   news.Summary,
			"analysis":  "",
			"sentiment": news.Sentiment,
		}
	}

	prompt := fmt.Sprintf(`你是专业金融新闻分析师。对以下与"%s"（%s）相关的新闻进行深度分析。

新闻标题：%s
新闻摘要：%s
新闻来源：%s

请提供：
1. summary（80-150字中文）：核心信息概括，包含事件主体、关键数据、直接影响
2. analysis（150-250字中文）：深度解读，包含背景原因、对该标的短中期影响、风险提示

严格按JSON格式返回：
{"summary":"...","analysis":"..."}`, stockName, stockCode, news.Title, news.Summary, news.Source)

	aiCtx, aiCancel := context.WithTimeout(ctx, 30*time.Second)
	defer aiCancel()

	resp, err := h.llmClient.CreateChatCompletion(aiCtx, llm.ChatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "你是金融新闻分析助手，只返回JSON格式数据，尽量精炼。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   1500,
	})
	if err != nil {
		h.logger.WithField("error", err).Warn("LLM enhancement failed for single news")
		return gin.H{
			"summary":   news.Summary,
			"analysis":  "",
			"sentiment": news.Sentiment,
		}
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		Summary  string `json:"summary"`
		Analysis string `json:"analysis"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		h.logger.WithField("error", err).Warn("Failed to parse single news enhancement")
		return gin.H{
			"summary":   news.Summary,
			"analysis":  "",
			"sentiment": news.Sentiment,
		}
	}

	return gin.H{
		"summary":   result.Summary,
		"analysis":  result.Analysis,
		"sentiment": news.Sentiment, // Preserve original sentiment from classifySentiments()
	}
}

// updateCachedNewsItem updates a single news item in the cache with AI enhancement.
func (h *StockHandler) updateCachedNewsItem(code, market, newsID string, enhanced gin.H) {
	key := getNewsCacheKey(code, market)
	newsMemCacheMu.Lock()
	defer newsMemCacheMu.Unlock()
	entry, ok := newsMemCache[key]
	if !ok {
		return
	}
	for i := range entry.data {
		if entry.data[i].ID == newsID {
			if s, ok := enhanced["summary"].(string); ok && s != "" {
				entry.data[i].Summary = s
			}
			if a, ok := enhanced["analysis"].(string); ok && a != "" {
				entry.data[i].Analysis = a
			}
			if s, ok := enhanced["sentiment"].(string); ok && s != "" {
				entry.data[i].Sentiment = s
			}
			break
		}
	}
}

// ── Batch News — POST /api/v1/stocks/news/batch ─────────────────────────────

type batchNewsRequest struct {
	Stocks []struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Market string `json:"market"`
	} `json:"stocks"`
}

type batchNewsResponseItem struct {
	Code   string         `json:"code"`
	Market string         `json:"market"`
	News   []NewsResponse `json:"news"`
}

// GetBatchNews fetches news for multiple stocks in parallel.
// Already-cached stocks are served from memory; only uncached ones hit the network.
func (h *StockHandler) GetBatchNews(c *gin.Context) {
	var req batchNewsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.Stocks) == 0 {
		c.JSON(http.StatusOK, []batchNewsResponseItem{})
		return
	}
	// Cap at 30 stocks per batch to avoid overloading
	if len(req.Stocks) > 30 {
		req.Stocks = req.Stocks[:30]
	}

	results := make([]batchNewsResponseItem, len(req.Stocks))
	var wg sync.WaitGroup

	// Limit concurrency to 6 parallel fetches
	sem := make(chan struct{}, 6)

	for i, s := range req.Stocks {
		code := s.Code
		name := s.Name
		market := s.Market
		if market == "" {
			market = "a_share"
		}

		// Normalize crypto params so market-overview and watchlist share the same code/name/cache
		if market == "crypto" {
			code, name = normalizeCryptoParams(code, name)
		}

		// Check server-side cache first
		if cached, ok := getNewsFromMemCache(code, market); ok {
			results[i] = batchNewsResponseItem{Code: code, Market: market, News: cached}
			continue
		}

		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(c.Request.Context(), 18*time.Second)
			defer cancel()

			news, err := h.fetchNewsViaSearch(ctx, code, name, market)
			if err != nil || len(news) == 0 {
				switch market {
				case "a_share":
					news, _ = h.fetchAShareNews(ctx, code, name)
				case "us_stock":
					news, _ = h.fetchUSStockNews(ctx, code, name)
				}
			}
			if news == nil {
				news = []NewsResponse{}
			}
			if len(news) > 0 {
				setNewsToMemCache(code, market, news)
			}
			results[idx] = batchNewsResponseItem{Code: code, Market: market, News: news}
		}()
	}

	wg.Wait()
	c.JSON(http.StatusOK, results)
}

// fetchNewsViaSearch uses the web search engine (Serper/Brave) to find recent news,
// then uses the LLM to generate AI summaries with sentiment analysis.
// It performs multiple search rounds to ensure 5-10 results:
//  1. Site-restricted search for the specific stock/coin
//  2. Broader market news search (for US stocks and crypto, captures policy/macro events)
//  3. Unrestricted search as fallback if still under 5 results
func (h *StockHandler) fetchNewsViaSearch(ctx context.Context, code, name, market string) ([]NewsResponse, error) {
	if h.searcher == nil {
		return nil, fmt.Errorf("searcher not configured")
	}

	displayName := name
	if displayName == "" {
		displayName = code
	}

	// Build site clause for quality domains
	domains, hasDomains := qualityDomains[market]
	siteClause := ""
	if hasDomains && len(domains) > 0 {
		parts := make([]string, len(domains))
		for i, d := range domains {
			parts[i] = "site:" + d
		}
		siteClause = " (" + strings.Join(parts, " OR ") + ")"
	}

	// --- Build search queries ---
	type searchRound struct {
		query       string
		maxResults  int
		allowBroad  bool // if true, relax title-match filter to allow market-wide news
	}
	var rounds []searchRound

	switch market {
	case "a_share":
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s 最新新闻 公告 分析", displayName) + siteClause,
			maxResults: 15,
		})
		// Supplementary: company announcements from cninfo
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s 公告 财报 site:cninfo.com.cn OR site:eastmoney.com OR site:10jqka.com.cn", displayName),
			maxResults: 8,
		})

	case "us_stock":
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s 美股 最新新闻 分析", displayName) + siteClause,
			maxResults: 15,
		})
		// Supplementary: broad market/policy news from jin10 + wallstreetcn that affects this stock
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s 最新消息 (site:jin10.com OR site:wallstreetcn.com OR site:cls.cn OR site:xueqiu.com)", displayName),
			maxResults: 10,
			allowBroad: true,
		})

	case "crypto":
		// Use Chinese name for better search relevance
		cnName := displayName
		if cn, ok := cryptoChineseNames[displayName]; ok {
			cnName = cn
		}
		if cn, ok := cryptoChineseNames[code]; ok && cnName == displayName {
			cnName = cn
		}
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s %s 最新新闻动态", cnName, displayName) + siteClause,
			maxResults: 15,
		})
		// Supplementary: broad crypto market news (policy, regulation, macro)
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s 加密货币 最新消息 (site:jin10.com OR site:wallstreetcn.com OR site:cls.cn OR site:jinse.cn OR site:theblockbeats.info)", cnName),
			maxResults: 10,
			allowBroad: true,
		})

	default:
		rounds = append(rounds, searchRound{
			query:      fmt.Sprintf("%s 最新新闻报道", displayName) + siteClause,
			maxResults: 15,
		})
	}

	// --- Execute search rounds concurrently ---
	type roundResult struct {
		results    []search.Result
		allowBroad bool
	}
	roundResults := make([]roundResult, len(rounds))
	var wg sync.WaitGroup
	for i, round := range rounds {
		wg.Add(1)
		go func(idx int, r searchRound) {
			defer wg.Done()
			res, err := h.searcher.Search(ctx, r.query, r.maxResults)
			if err != nil {
				h.logger.WithField("error", err).WithField("query", r.query).Debug("Search round failed")
				return
			}
			roundResults[idx] = roundResult{results: res, allowBroad: r.allowBroad}
		}(i, round)
	}
	wg.Wait()

	// --- Merge & deduplicate results, applying filters ---
	const maxNews = 10
	seen := make(map[string]bool) // deduplicate by URL
	news := make([]NewsResponse, 0, maxNews)

	// Crypto Chinese name for title matching
	cryptoCNName := ""
	if market == "crypto" {
		if cn, ok := cryptoChineseNames[displayName]; ok {
			cryptoCNName = cn
		}
		if cryptoCNName == "" {
			if cn, ok := cryptoChineseNames[code]; ok {
				cryptoCNName = cn
			}
		}
	}

	for _, rr := range roundResults {
		for ri, r := range rr.results {
			if len(news) >= maxNews {
				break
			}
			// Deduplicate
			urlKey := strings.ToLower(r.URL)
			if seen[urlKey] {
				continue
			}
			// Skip homepage/category/stock-quote URLs
			if isNonArticleURL(r.URL, code) {
				continue
			}
			titleLower := strings.ToLower(r.Title)
			// Skip stock quote/profile page titles
			if isQuotePageTitle(titleLower) {
				continue
			}
			// Title relevance check
			if !h.isTitleRelevant(titleLower, displayName, code, market, cryptoCNName, rr.allowBroad) {
				continue
			}

			seen[urlKey] = true
			source := extractDomain(r.URL)
			newsDate := normalizeSearchDate(r.Date)
			news = append(news, NewsResponse{
				ID:        fmt.Sprintf("%s_search_%d_%d", code, len(news), ri),
				Title:     r.Title,
				Source:    source,
				Time:      newsDate,
				Summary:   r.Snippet,
				Sentiment: "neutral",
				URL:       r.URL,
			})
		}
	}

	// --- Fallback: if still under 5 results, do an unrestricted search ---
	if len(news) < 5 && h.searcher != nil {
		var fallbackQuery string
		switch market {
		case "a_share":
			fallbackQuery = fmt.Sprintf("%s 股票 最新新闻", displayName)
		case "us_stock":
			fallbackQuery = fmt.Sprintf("%s 美股 新闻 最新", displayName)
		case "crypto":
			cnName := displayName
			if cn, ok := cryptoChineseNames[displayName]; ok {
				cnName = cn
			}
			fallbackQuery = fmt.Sprintf("%s %s 新闻 最新", cnName, displayName)
		default:
			fallbackQuery = fmt.Sprintf("%s 最新新闻", displayName)
		}
		fbResults, fbErr := h.searcher.Search(ctx, fallbackQuery, 15)
		if fbErr == nil {
			for ri, r := range fbResults {
				if len(news) >= maxNews {
					break
				}
				urlKey := strings.ToLower(r.URL)
				if seen[urlKey] {
					continue
				}
				if isNonArticleURL(r.URL, code) {
					continue
				}
				if isQuotePageTitle(strings.ToLower(r.Title)) {
					continue
				}
				// Fallback round: relaxed title matching — allow broader results
				if !h.isTitleRelevant(strings.ToLower(r.Title), displayName, code, market, cryptoCNName, true) {
					continue
				}
				seen[urlKey] = true
				source := extractDomain(r.URL)
				newsDate := normalizeSearchDate(r.Date)
				news = append(news, NewsResponse{
					ID:        fmt.Sprintf("%s_fb_%d_%d", code, len(news), ri),
					Title:     r.Title,
					Source:    source,
					Time:      newsDate,
					Summary:   r.Snippet,
					Sentiment: "neutral",
					URL:       r.URL,
				})
			}
		}
	}

	// Sort news by date descending (newest first)
	sort.Slice(news, func(i, j int) bool {
		return news[i].Time > news[j].Time
	})

	// Synchronous lightweight sentiment classification — so the first response already
	// has accurate sentiment (positive/negative/neutral) for each news item.
	// This prevents the UI from showing "neutral" initially and then flipping to "positive"
	// when the user opens the detail page.
	if h.llmClient != nil && len(news) > 0 {
		sentCtx, sentCancel := context.WithTimeout(ctx, 12*time.Second)
		defer sentCancel()
		h.classifySentiments(sentCtx, news, name, code)
	}

	return news, nil
}

// isQuotePageTitle checks if a news title is actually a stock quote/profile page.
func isQuotePageTitle(titleLower string) bool {
	quotePageKeywords := []string{
		"股票价格", "股价", "实时行情", "今日行情", "stock price",
		"实时股价", "股票行情", "即时行情", "个股主页", "行情走势",
		"行情数据", "实时数据", "交易数据", "行情中心",
	}
	for _, kw := range quotePageKeywords {
		if strings.Contains(titleLower, kw) {
			return true
		}
	}
	return false
}

// isTitleRelevant checks if a news title is relevant to the given stock/coin.
// When allowBroad is true, it also accepts general market/policy news.
func (h *StockHandler) isTitleRelevant(titleLower, displayName, code, market, cryptoCNName string, allowBroad bool) bool {
	nameLower := strings.ToLower(displayName)
	codeLower := strings.ToLower(code)

	// Direct match: title contains stock name or code
	if strings.Contains(titleLower, nameLower) || strings.Contains(titleLower, codeLower) {
		return true
	}

	// For short names (<=2 chars), always allow through
	if len([]rune(displayName)) <= 2 {
		return true
	}

	// Crypto: check Chinese name
	if market == "crypto" && cryptoCNName != "" {
		if strings.Contains(titleLower, strings.ToLower(cryptoCNName)) {
			return true
		}
	}

	// Crypto: general crypto keywords always pass
	if market == "crypto" {
		cryptoGenericKeywords := []string{"加密货币", "币圈", "数字货币", "虚拟货币", "crypto", "web3", "defi"}
		for _, kw := range cryptoGenericKeywords {
			if strings.Contains(titleLower, kw) {
				return true
			}
		}
	}

	// Broad mode: accept market-wide news (policy, macro events)
	if allowBroad {
		broadKeywords := []string{
			// Political / policy figures
			"特朗普", "trump", "拜登", "biden", "美联储", "fed ", "sec ",
			"鲍威尔", "powell", "耶伦", "yellen",
			// Market-wide events
			"关税", "tariff", "制裁", "sanction", "贸易战",
			"降息", "加息", "利率", "通胀", "cpi", "非农",
			// Market indices
			"纳斯达克", "nasdaq", "标普", "s&p", "道琼斯", "dow jones",
			"大涨", "大跌", "暴涨", "暴跌", "崩盘", "飙升", "熔断",
		}
		if market == "crypto" {
			broadKeywords = append(broadKeywords,
				"监管", "etf", "合规", "交易所", "binance", "coinbase",
				"稳定币", "usdt", "usdc", "挖矿", "减半", "halving",
			)
		}
		if market == "us_stock" {
			broadKeywords = append(broadKeywords,
				"财报", "earnings", "科技股", "芯片", "ai ", "人工智能",
				"美股", "华尔街", "wall street",
			)
		}
		for _, kw := range broadKeywords {
			if strings.Contains(titleLower, kw) {
				return true
			}
		}
	}

	return false
}

// classifySentiments is a lightweight LLM call that only classifies sentiment for news items.
// It runs synchronously before the news list is returned so sentiment is accurate from the start.
// Token usage is minimal (~200-400 tokens) because it only outputs sentiment labels.
func (h *StockHandler) classifySentiments(ctx context.Context, news []NewsResponse, stockName, stockCode string) {
	if h.llmClient == nil || len(news) == 0 {
		return
	}

	var sb strings.Builder
	for i, n := range news {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, n.Title))
	}

	prompt := fmt.Sprintf(`对以下与"%s"（%s）相关的新闻标题进行情绪分类。
每条只需判断：positive（利好）、negative（利空）或 neutral（中性）。

新闻标题：
%s
严格按JSON数组格式返回，例如：["positive","neutral","negative",...]
数组长度必须等于新闻数量（%d条）。`, stockName, stockCode, sb.String(), len(news))

	resp, err := h.llmClient.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "你是金融新闻情绪分析助手，只返回JSON数组。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   200,
	})
	if err != nil {
		h.logger.WithField("error", err).Debug("Sentiment classification failed")
		return
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var sentiments []string
	if err := json.Unmarshal([]byte(content), &sentiments); err != nil {
		h.logger.WithField("error", err).Debug("Failed to parse sentiment classification")
		return
	}

	for i, s := range sentiments {
		if i < len(news) && (s == "positive" || s == "negative" || s == "neutral") {
			news[i].Sentiment = s
		}
	}
}

// ── Batch Enhance News — POST /api/v1/stocks/news/enhance/batch ─────────────
// Called when user enters a market section (e.g. A股). Triggers AI summary+analysis
// for all watchlist stocks' news in that market. Does NOT change sentiment (already set).

type batchEnhanceRequest struct {
	Stocks []struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Market string `json:"market"`
	} `json:"stocks"`
}

func (h *StockHandler) EnhanceBatchNews(c *gin.Context) {
	var req batchEnhanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.Stocks) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "enhanced": 0})
		return
	}
	if len(req.Stocks) > 30 {
		req.Stocks = req.Stocks[:30]
	}

	// Count how many stocks need enhancement
	type enhanceJob struct {
		code, name, market string
		cached             []NewsResponse
	}
	var jobs []enhanceJob

	for _, s := range req.Stocks {
		code := s.Code
		name := s.Name
		market := s.Market
		if market == "" {
			market = "a_share"
		}
		if market == "crypto" {
			code, name = normalizeCryptoParams(code, name)
		}

		cached, ok := getNewsFromMemCache(code, market)
		if !ok || len(cached) == 0 {
			continue
		}

		// Check if already enhanced (first item has analysis)
		if cached[0].Analysis != "" {
			continue
		}

		jobs = append(jobs, enhanceJob{code: code, name: name, market: market, cached: cached})
	}

	// Return immediately — enhancement runs asynchronously in the background.
	// Each stock's cache is updated as soon as its LLM call completes,
	// so the client can poll via GetBatchNews to pick up results incrementally.
	h.logger.Info("BatchEnhance: accepted %d jobs, starting async processing", len(jobs))
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "pending": len(jobs)})

	// Fire-and-forget: process all jobs in background goroutines
	batchStart := time.Now()
	sem := make(chan struct{}, 6) // Limit concurrency for LLM calls
	for _, job := range jobs {
		go func(j enhanceJob) {
			sem <- struct{}{}
			defer func() { <-sem }()

			jobStart := time.Now()
			enhCtx, enhCancel := context.WithTimeout(context.Background(), 65*time.Second)
			defer enhCancel()

			// Make a copy to avoid race conditions
			newsCopy := make([]NewsResponse, len(j.cached))
			copy(newsCopy, j.cached)

			h.enhanceNewsWithAI(enhCtx, newsCopy, j.name, j.code)
			setNewsToMemCache(j.code, j.market, newsCopy)
			h.logger.Info("BatchEnhance: completed %s (%s) in %.1fs (total elapsed %.1fs)",
				j.code, j.name, time.Since(jobStart).Seconds(), time.Since(batchStart).Seconds())
		}(job)
	}
}

// enhanceNewsWithAI uses the LLM to generate summaries and analysis for news items.
// NOTE: This does NOT update sentiment — sentiment is already set by classifySentiments()
// during the initial news fetch.
func (h *StockHandler) enhanceNewsWithAI(ctx context.Context, news []NewsResponse, stockName, stockCode string) {
	// Build context for LLM
	var sb strings.Builder
	for i, n := range news {
		sb.WriteString(fmt.Sprintf("【新闻%d】标题：%s\n摘要：%s\n来源：%s\n\n", i+1, n.Title, n.Summary, n.Source))
	}

	prompt := fmt.Sprintf(`你是专业金融新闻分析师。对以下与"%s"（%s）相关的新闻进行分析。

每条新闻请提供：
1. summary（80-150字中文）：核心信息概括，包含事件主体、关键数据、直接影响
2. analysis（150-250字中文）：深度解读，包含背景原因、对该标的短中期影响、风险提示

严格按JSON格式返回：
[{"index":0,"summary":"...","analysis":"..."}, ...]

%s`, stockName, stockCode, sb.String())

	aiCtx, aiCancel := context.WithTimeout(ctx, 60*time.Second)
	defer aiCancel()

	resp, err := h.llmClient.CreateChatCompletion(aiCtx, llm.ChatCompletionRequest{
		Model: "deepseek-chat",
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "你是金融新闻分析助手，只返回JSON格式数据，尽量精炼。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   3000,
	})
	if err != nil {
		h.logger.WithField("error", err).Warn("LLM enhancement failed for news")
		return
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var enhancements []struct {
		Index    int    `json:"index"`
		Summary  string `json:"summary"`
		Analysis string `json:"analysis"`
	}
	if err := json.Unmarshal([]byte(content), &enhancements); err != nil {
		h.logger.WithField("error", err).WithField("content", content).Warn("Failed to parse LLM news enhancement")
		return
	}

	for _, e := range enhancements {
		if e.Index >= 0 && e.Index < len(news) {
			if e.Summary != "" {
				news[e.Index].Summary = e.Summary
			}
			if e.Analysis != "" {
				news[e.Index].Analysis = e.Analysis
			}
			// NOTE: Do NOT overwrite Sentiment — it was already set by classifySentiments()
		}
	}
}

// extractDomain extracts a readable domain name from a URL.
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "网络"
	}
	host := u.Hostname()
	// Remove www. prefix
	host = strings.TrimPrefix(host, "www.")

	// Map common domains to Chinese names
	domainMap := map[string]string{
		"finance.sina.com.cn":    "新浪财经",
		"finance.qq.com":         "腾讯财经",
		"stock.eastmoney.com":    "东方财富",
		"eastmoney.com":          "东方财富",
		"xueqiu.com":             "雪球",
		"wallstreetcn.com":       "华尔街见闻",
		"cls.cn":                 "财联社",
		"caixin.com":             "财新",
		"36kr.com":               "36氪",
		"jinse.cn":               "金色财经",
		"jinse.com":              "金色财经",
		"8btc.com":               "巴比特",
		"theblockbeats.info":     "BlockBeats",
		"odaily.news":            "Odaily星球日报",
		"panewslab.com":          "PANews",
		"techflowpost.com":       "深潮TechFlow",
		"bloomberg.com":          "Bloomberg",
		"reuters.com":            "Reuters",
		"cnbc.com":               "CNBC",
		"wsj.com":                "华尔街日报",
		"nbd.com.cn":             "每日经济新闻",
		"stcn.com":               "证券时报",
		"cs.com.cn":              "中证网",
		"10jqka.com.cn":          "同花顺",
		"yicai.com":              "第一财经",
		"thepaper.cn":            "澎湃新闻",
		"techcrunch.com":         "TechCrunch",
		"jin10.com":              "金十数据",
		"cninfo.com.cn":          "巨潮资讯",
		"foresightnews.pro":      "Foresight News",
		"marsbit.co":             "MarsBit",
	}

	for domain, cnName := range domainMap {
		if strings.Contains(host, domain) {
			return cnName
		}
	}

	// Return the domain itself for unknown sources
	return host
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
