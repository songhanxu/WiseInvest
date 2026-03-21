package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// StockHandler provides real-time market data endpoints.
type StockHandler struct {
	watchlistRepo *repository.WatchlistRepository
	logger        *logger.Logger
	httpClient    *http.Client
}

// NewStockHandler creates a new StockHandler.
func NewStockHandler(watchlistRepo *repository.WatchlistRepository, logger *logger.Logger) *StockHandler {
	return &StockHandler{
		watchlistRepo: watchlistRepo,
		logger:        logger,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
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
		h.logger.WithField("error", err).Warn("Failed to fetch indices, returning empty")
		indices = []IndexResponse{}
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

	// Fetch sparkline data for each index (minute-level trend)
	for i, idx := range results {
		sparkline, err := h.fetchTencentMinuteData(ctx, idx.ID)
		if err == nil && len(sparkline) > 0 {
			results[i].SparklineData = sparkline
		}
	}

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
	symbols := []struct {
		symbol    string
		name      string
		shortName string
	}{
		{"^DJI", "道琼斯工业平均指数", "道指"},
		{"^GSPC", "标普500指数", "标普"},
		{"^IXIC", "纳斯达克综合指数", "纳指"},
	}

	var results []IndexResponse
	for _, s := range symbols {
		idx, err := h.fetchYahooQuote(ctx, s.symbol, s.name, s.shortName)
		if err != nil {
			h.logger.WithField("error", err).Warnf("Failed to fetch %s", s.symbol)
			continue
		}
		results = append(results, idx)
	}
	return results, nil
}

func (h *StockHandler) fetchYahooQuote(ctx context.Context, symbol, name, shortName string) (IndexResponse, error) {
	apiURL := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=5d",
		url.PathEscape(symbol),
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return IndexResponse{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return IndexResponse{}, err
	}
	defer resp.Body.Close()

	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice float64 `json:"regularMarketPrice"`
					PreviousClose      float64 `json:"previousClose"`
				} `json:"meta"`
				Indicators struct {
					Quote []struct {
						Close []interface{} `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return IndexResponse{}, err
	}
	if len(payload.Chart.Result) == 0 {
		return IndexResponse{}, fmt.Errorf("no data for %s", symbol)
	}

	m := payload.Chart.Result[0].Meta
	change := m.RegularMarketPrice - m.PreviousClose
	changePct := 0.0
	if m.PreviousClose > 0 {
		changePct = change / m.PreviousClose * 100
	}

	// Extract sparkline from close prices
	var sparkline []float64
	if len(payload.Chart.Result[0].Indicators.Quote) > 0 {
		for _, v := range payload.Chart.Result[0].Indicators.Quote[0].Close {
			if f, ok := v.(float64); ok && f > 0 {
				sparkline = append(sparkline, f)
			}
		}
	}

	id := strings.ReplaceAll(symbol, "^", "")
	return IndexResponse{
		ID:            id,
		Name:          name,
		ShortName:     shortName,
		Value:         m.RegularMarketPrice,
		Change:        change,
		ChangePercent: changePct,
		SparklineData: sparkline,
	}, nil
}

// ── Crypto Indices (CoinGecko) ───────────────────────────────────────────────

func (h *StockHandler) fetchCryptoIndices(ctx context.Context) ([]IndexResponse, error) {
	apiURL := "https://api.coingecko.com/api/v3/global"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			TotalMarketCap           map[string]float64 `json:"total_market_cap"`
			MarketCapChangePerc24h   float64            `json:"market_cap_change_percentage_24h_usd"`
			BTCDominance             float64            `json:"market_cap_percentage"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	totalCap := data.Data.TotalMarketCap["usd"] / 1e12 // in trillions
	totalCapChange := data.Data.MarketCapChangePerc24h

	// Fetch BTC and overall sentiment from CoinGecko simple price
	btcData, _ := h.fetchCoinGeckoPrices(ctx, "bitcoin")
	var btcDominance float64
	if data.Data.BTCDominance > 0 {
		btcDominance = data.Data.BTCDominance
	}

	indices := []IndexResponse{
		{
			ID:            "total_mcap",
			Name:          "加密总市值",
			ShortName:     "总市值",
			Value:         totalCap,
			Change:        totalCap * totalCapChange / 100,
			ChangePercent: totalCapChange,
			SparklineData: []float64{},
		},
		{
			ID:            "btc_dom",
			Name:          "BTC 主导率",
			ShortName:     "BTC.D",
			Value:         btcDominance,
			Change:        0,
			ChangePercent: 0,
			SparklineData: []float64{},
		},
	}

	// Fear & Greed index from alternative.me
	fgIdx, err := h.fetchFearGreedIndex(ctx)
	if err == nil {
		indices = append(indices, fgIdx)
	}

	_ = btcData
	return indices, nil
}

func (h *StockHandler) fetchCoinGeckoPrices(ctx context.Context, ids string) (map[string]map[string]float64, error) {
	apiURL := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true&include_24hr_vol=true&include_market_cap=true",
		ids,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
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

	// Yahoo Finance autocomplete API
	apiURL := fmt.Sprintf("https://query1.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=10&newsCount=0", url.QueryEscape(query))
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

	var searchResp struct {
		Quotes []struct {
			Symbol   string `json:"symbol"`
			ShortName string `json:"shortname"`
			Exchange  string `json:"exchange"`
		} `json:"quotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	var symbols []string
	for _, q := range searchResp.Quotes {
		// Filter to US exchanges only
		if q.Exchange == "NMS" || q.Exchange == "NYQ" || q.Exchange == "NGM" || q.Exchange == "NAS" || q.Exchange == "NYSE" {
			symbols = append(symbols, q.Symbol)
		}
	}

	if len(symbols) == 0 {
		return []StockResponse{}, nil
	}

	return h.fetchUSStocksBySymbol(ctx, strings.Join(symbols, ","))
}

func (h *StockHandler) fetchUSStocksBySymbol(ctx context.Context, symbolsStr string) ([]StockResponse, error) {
	symbols := strings.Split(symbolsStr, ",")
	var stocks []StockResponse

	for _, sym := range symbols {
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		apiURL := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", sym)
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := h.httpClient.Do(req)
		if err != nil {
			continue
		}

		var payload struct {
			Chart struct {
				Result []struct {
					Meta struct {
						Symbol             string  `json:"symbol"`
						RegularMarketPrice float64 `json:"regularMarketPrice"`
						PreviousClose      float64 `json:"previousClose"`
						RegularMarketOpen  float64 `json:"regularMarketOpen"`
						RegularMarketVolume int64  `json:"regularMarketVolume"`
						RegularMarketDayHigh float64 `json:"regularMarketDayHigh"`
						RegularMarketDayLow  float64 `json:"regularMarketDayLow"`
						ShortName          string  `json:"shortName"`
					} `json:"meta"`
				} `json:"result"`
			} `json:"chart"`
		}
		json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()

		if len(payload.Chart.Result) == 0 {
			continue
		}

		m := payload.Chart.Result[0].Meta
		change := m.RegularMarketPrice - m.PreviousClose
		changePct := 0.0
		if m.PreviousClose > 0 {
			changePct = change / m.PreviousClose * 100
		}

		displayName := m.ShortName
		if displayName == "" {
			displayName = m.Symbol
		}

		stocks = append(stocks, StockResponse{
			ID:            m.Symbol,
			Symbol:        m.Symbol,
			Name:          displayName,
			Market:        "us_stock",
			CurrentPrice:  m.RegularMarketPrice,
			Change:        change,
			ChangePercent: changePct,
			Volume:        float64(m.RegularMarketVolume) / 1e8, // to 亿股
			High:          m.RegularMarketDayHigh,
			Low:           m.RegularMarketDayLow,
			Open:          m.RegularMarketOpen,
			PreviousClose: m.PreviousClose,
		})
	}
	return stocks, nil
}

// ── Crypto Search (CoinGecko) ────────────────────────────────────────────────

var defaultCryptoIDs = map[string]struct {
	Symbol string
	Name   string
}{
	"bitcoin":      {"BTC/USDT", "Bitcoin"},
	"ethereum":     {"ETH/USDT", "Ethereum"},
	"solana":       {"SOL/USDT", "Solana"},
	"binancecoin":  {"BNB/USDT", "BNB"},
	"ripple":       {"XRP/USDT", "XRP"},
	"dogecoin":     {"DOGE/USDT", "Dogecoin"},
	"cardano":      {"ADA/USDT", "Cardano"},
	"avalanche-2":  {"AVAX/USDT", "Avalanche"},
}

var cryptoSymbolToID = map[string]string{
	"btc": "bitcoin", "eth": "ethereum", "bnb": "binancecoin", "sol": "solana",
	"xrp": "ripple", "ada": "cardano", "doge": "dogecoin", "avax": "avalanche-2",
	"dot": "polkadot", "link": "chainlink", "ltc": "litecoin", "atom": "cosmos",
}

func (h *StockHandler) searchCryptoStocks(ctx context.Context, query string) ([]StockResponse, error) {
	ids := "bitcoin,ethereum,solana,binancecoin,ripple,dogecoin,cardano,avalanche-2"
	if query != "" {
		q := strings.ToLower(strings.TrimSpace(query))
		if cgID, ok := cryptoSymbolToID[q]; ok {
			ids = cgID
		} else {
			// Use CoinGecko search
			searchURL := fmt.Sprintf("https://api.coingecko.com/api/v3/search?query=%s", url.QueryEscape(q))
			req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := h.httpClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			var searchResp struct {
				Coins []struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Symbol string `json:"symbol"`
				} `json:"coins"`
			}
			json.NewDecoder(resp.Body).Decode(&searchResp)

			var coinIDs []string
			for _, c := range searchResp.Coins {
				if len(coinIDs) >= 10 {
					break
				}
				coinIDs = append(coinIDs, c.ID)
			}
			if len(coinIDs) > 0 {
				ids = strings.Join(coinIDs, ",")
			}
		}
	}

	prices, err := h.fetchCoinGeckoPrices(ctx, ids)
	if err != nil {
		return nil, err
	}

	var stocks []StockResponse
	for id, data := range prices {
		price := data["usd"]
		change24h := data["usd_24h_change"]
		vol := data["usd_24h_vol"]
		mcap := data["usd_market_cap"]

		info, ok := defaultCryptoIDs[id]
		symbol := strings.ToUpper(id) + "/USDT"
		name := strings.ToUpper(id)
		if ok {
			symbol = info.Symbol
			name = info.Name
		}

		// Approximate previous close from 24h change
		prevPrice := price / (1 + change24h/100)

		stocks = append(stocks, StockResponse{
			ID:            strings.ToUpper(strings.Split(symbol, "/")[0]),
			Symbol:        symbol,
			Name:          name,
			Market:        "crypto",
			CurrentPrice:  price,
			Change:        price - prevPrice,
			ChangePercent: change24h,
			Volume:        vol / 1e8,
			High:          price * 1.01, // CoinGecko simple API doesn't provide high/low
			Low:           price * 0.99,
			Open:          prevPrice,
			PreviousClose: prevPrice,
		})
		_ = mcap
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
			prefix := "sh"
			if strings.HasPrefix(item.StockCode, "0") || strings.HasPrefix(item.StockCode, "3") {
				prefix = "sz"
			}
			codes = append(codes, prefix+item.StockCode)
		}
		stocks, _ = h.fetchAShareStocksByCode(ctx, strings.Join(codes, ","))
	case "us_stock":
		var symbols []string
		for _, item := range items {
			symbols = append(symbols, item.StockCode)
		}
		stocks, _ = h.fetchUSStocksBySymbol(ctx, strings.Join(symbols, ","))
	case "crypto":
		var ids []string
		for _, item := range items {
			id := strings.ToLower(item.StockCode)
			if cgID, ok := cryptoSymbolToID[id]; ok {
				ids = append(ids, cgID)
			} else {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			prices, err := h.fetchCoinGeckoPrices(ctx, strings.Join(ids, ","))
			if err == nil {
				for id, data := range prices {
					price := data["usd"]
					change24h := data["usd_24h_change"]
					vol := data["usd_24h_vol"]
					prevPrice := price / (1 + change24h/100)
					info, ok := defaultCryptoIDs[id]
					symbol := strings.ToUpper(id) + "/USDT"
					name := strings.ToUpper(id)
					if ok {
						symbol = info.Symbol
						name = info.Name
					}
					stocks = append(stocks, StockResponse{
						ID:            strings.ToUpper(strings.Split(symbol, "/")[0]),
						Symbol:        symbol,
						Name:          name,
						Market:        "crypto",
						CurrentPrice:  price,
						Change:        price - prevPrice,
						ChangePercent: change24h,
						Volume:        vol / 1e8,
						High:          price * 1.01,
						Low:           price * 0.99,
						Open:          prevPrice,
						PreviousClose: prevPrice,
					})
				}
			}
		}
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

	item := &model.WatchlistItem{
		UserID:    userID,
		Market:    req.Market,
		StockCode: req.StockCode,
		Symbol:    req.Symbol,
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
		q := strings.ToLower(code)
		if cgID, ok := cryptoSymbolToID[q]; ok {
			q = cgID
		}
		prices, ferr := h.fetchCoinGeckoPrices(ctx, q)
		if ferr != nil {
			err = ferr
		} else {
			for id, data := range prices {
				price := data["usd"]
				change24h := data["usd_24h_change"]
				prevPrice := price / (1 + change24h/100)
				stocks = append(stocks, StockResponse{
					ID:            strings.ToUpper(code),
					Symbol:        strings.ToUpper(code) + "/USDT",
					Name:          strings.ToUpper(id),
					Market:        "crypto",
					CurrentPrice:  price,
					Change:        price - prevPrice,
					ChangePercent: change24h,
					PreviousClose: prevPrice,
				})
			}
		}
	}

	if err != nil || len(stocks) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "stock not found"})
		return
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
		h.logger.WithField("error", err).Warn("Failed to fetch K-line data")
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

func (h *StockHandler) fetchUSStockKLine(ctx context.Context, symbol string, days string) ([]KLineResponse, error) {
	daysInt := 60
	if d := parseFloat(days); d > 0 {
		daysInt = int(d)
	}

	range_ := "3mo"
	if daysInt > 90 {
		range_ = "6mo"
	}
	if daysInt > 180 {
		range_ = "1y"
	}

	apiURL := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=%s",
		url.PathEscape(symbol), range_,
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

	var payload struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []interface{} `json:"open"`
						Close  []interface{} `json:"close"`
						High   []interface{} `json:"high"`
						Low    []interface{} `json:"low"`
						Volume []interface{} `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("no data")
	}

	r := payload.Chart.Result[0]
	q := r.Indicators.Quote[0]
	var klines []KLineResponse

	for i := range r.Timestamp {
		if i >= len(q.Open) || i >= len(q.Close) || i >= len(q.High) || i >= len(q.Low) || i >= len(q.Volume) {
			break
		}

		open, _ := toFloatVal(q.Open[i])
		close, _ := toFloatVal(q.Close[i])
		high, _ := toFloatVal(q.High[i])
		low, _ := toFloatVal(q.Low[i])
		vol, _ := toFloatVal(q.Volume[i])

		if open == 0 && close == 0 {
			continue
		}

		t := time.Unix(r.Timestamp[i], 0)
		klines = append(klines, KLineResponse{
			Date:   t.Format("2006-01-02"),
			Open:   open,
			Close:  close,
			High:   high,
			Low:    low,
			Volume: vol,
		})
	}

	if len(klines) > daysInt {
		klines = klines[len(klines)-daysInt:]
	}
	return klines, nil
}

func (h *StockHandler) fetchCryptoKLine(ctx context.Context, coinID string, days string) ([]KLineResponse, error) {
	q := strings.ToLower(coinID)
	if cgID, ok := cryptoSymbolToID[q]; ok {
		q = cgID
	}

	daysStr := "60"
	if days != "" {
		daysStr = days
	}

	apiURL := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/coins/%s/ohlc?vs_currency=usd&days=%s",
		q, daysStr,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Response: [[timestamp, open, high, low, close], ...]
	var data [][]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var klines []KLineResponse
	for _, candle := range data {
		if len(candle) < 5 {
			continue
		}
		t := time.Unix(int64(candle[0])/1000, 0)
		klines = append(klines, KLineResponse{
			Date:   t.Format("2006-01-02"),
			Open:   candle[1],
			Close:  candle[4],
			High:   candle[2],
			Low:    candle[3],
			Volume: 0,
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
	// Yahoo Finance news via search
	query := symbol
	if name != "" {
		query = name + " stock"
	}
	apiURL := fmt.Sprintf("https://query1.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=0&newsCount=8", url.QueryEscape(query))
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

	var result struct {
		News []struct {
			Title     string `json:"title"`
			Publisher string `json:"publisher"`
			Link      string `json:"link"`
			ProviderPublishTime int64 `json:"providerPublishTime"`
		} `json:"news"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	var news []NewsResponse
	for i, n := range result.News {
		t := time.Unix(n.ProviderPublishTime, 0)
		timeStr := t.Format("01-02 15:04")
		news = append(news, NewsResponse{
			ID:        fmt.Sprintf("%s_news_%d", symbol, i),
			Title:     n.Title,
			Source:    n.Publisher,
			Time:      timeStr,
			Summary:   n.Title,
			Sentiment: "neutral",
			URL:       n.Link,
		})
	}
	return news, nil
}

func (h *StockHandler) fetchCryptoNews(ctx context.Context, coinID string, name string) ([]NewsResponse, error) {
	// CoinGecko doesn't have a free news API; use placeholder
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
