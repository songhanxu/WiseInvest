package handler

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
)

func validStockMarket(market string) bool {
	return market == "a_share" || market == "us_stock" || market == "hk_stock" || market == "crypto"
}

func bitgetStock(q bitget.Quote) StockResponse {
	return StockResponse{ID: q.Code, Symbol: q.Code, Name: q.Name, Market: q.Market, CurrentPrice: q.Price, Change: q.Change, ChangePercent: q.ChangePercent, Volume: q.Volume / 1e8, High: q.High, Low: q.Low, Open: q.Open, PreviousClose: q.Open, Source: "bitget", ProviderSymbol: q.ProviderSymbol, Currency: q.Currency, ProductType: q.ProductType, ChangeBasis: "24h", QuoteTime: q.Timestamp}
}

func (h *StockHandler) fetchBitgetStocks(ctx context.Context, market string, codes []string) ([]StockResponse, error) {
	quotes, err := h.bitget.Quotes(ctx, market, codes)
	if err != nil {
		return nil, err
	}
	rows := make([]StockResponse, 0, len(quotes))
	for _, q := range quotes {
		rows = append(rows, bitgetStock(q))
	}
	return rows, nil
}

// Bitget is primary; per-symbol fallbacks retain their own source, currency
// and exchange quote timestamp so the UI can distinguish sessions.
func (h *StockHandler) fetchUSStocksBySymbol(ctx context.Context, symbols string) ([]StockResponse, error) {
	codes := strings.Split(symbols, ",")
	primaryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	rows, _ := h.fetchBitgetStocks(primaryCtx, "us_stock", codes)
	cancel()
	found := make(map[string]bool)
	for _, row := range rows {
		found[row.ID] = true
	}
	var missing []string
	for _, code := range codes {
		code = bitget.NormalizeCode("us_stock", code)
		if code != "" && !found[code] {
			missing = append(missing, code)
		}
	}
	if len(missing) == 0 {
		return rows, nil
	}
	fallback, err := h.fetchTencentUSStocksBySymbol(ctx, strings.Join(missing, ","))
	for _, row := range fallback {
		row.Source = "tencent"
		row.Currency = "USD"
		row.ProductType = "equity"
		row.ChangeBasis = "previous_close"
		rows = append(rows, row)
	}
	if len(rows) > 0 {
		return rows, nil
	}
	return rows, err
}

func (h *StockHandler) searchUSStocks(ctx context.Context, query string) ([]StockResponse, error) {
	primaryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	quotes, err := h.bitget.Search(primaryCtx, "us_stock", query)
	cancel()
	if err == nil && len(quotes) > 0 {
		rows := make([]StockResponse, 0, len(quotes))
		for _, q := range quotes {
			rows = append(rows, bitgetStock(q))
		}
		return rows, nil
	}
	rows, fallbackErr := h.searchTencentUSStocks(ctx, query)
	for i := range rows {
		rows[i].Source = "tencent"
		rows[i].Currency = "USD"
		rows[i].ProductType = "equity"
		rows[i].ChangeBasis = "previous_close"
	}
	return rows, fallbackErr
}

func (h *StockHandler) searchHKStocks(ctx context.Context, query string) ([]StockResponse, error) {
	primaryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	quotes, err := h.bitget.Search(primaryCtx, "hk_stock", query)
	cancel()
	if err != nil || len(quotes) == 0 {
		return h.searchTencentHKStocks(ctx, query)
	}
	rows := make([]StockResponse, 0, len(quotes))
	for _, q := range quotes {
		rows = append(rows, bitgetStock(q))
	}
	return rows, nil
}

// HK quotes use Bitget first, then preserve Tencent exchange timestamps.
func (h *StockHandler) fetchHKStocksByCode(ctx context.Context, codes []string) ([]StockResponse, error) {
	primaryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	rows, _ := h.fetchBitgetStocks(primaryCtx, "hk_stock", codes)
	cancel()
	found := map[string]bool{}
	for _, row := range rows {
		found[row.ID] = true
	}
	var missing []string
	for _, code := range codes {
		code = bitget.NormalizeCode("hk_stock", code)
		if !found[code] {
			missing = append(missing, code)
		}
	}
	if len(missing) == 0 {
		return rows, nil
	}
	fallback, err := h.fetchTencentHKStocks(ctx, missing)
	rows = append(rows, fallback...)
	if len(rows) > 0 {
		return rows, nil
	}
	return rows, err
}
func (h *StockHandler) fetchHKStockBoard(ctx context.Context) ([]IndexResponse, error) {
	return h.fetchBitgetMarketBoard(ctx, "hk_stock", []string{"00700", "01810", "00992"})
}

func (h *StockHandler) fetchBitgetKLine(ctx context.Context, market, code, period string, limit int) ([]KLineResponse, error) {
	candles, err := h.bitget.Candles(ctx, market, code, period, limit)
	if err != nil {
		return nil, err
	}
	rows := make([]KLineResponse, 0, len(candles))
	for _, c := range candles {
		date := c.Time.Format(time.RFC3339)
		if period == "1d" || period == "1w" {
			date = c.Time.Format("2006-01-02")
		}
		rows = append(rows, KLineResponse{Date: date, Open: c.Open, Close: c.Close, High: c.High, Low: c.Low, Volume: c.Volume, Source: "bitget"})
	}
	return rows, nil
}

func (h *StockHandler) fetchUSStockKLine(ctx context.Context, code, period string, limit int) ([]KLineResponse, error) {
	if _, isIndex := usIndexTencentCode[strings.ToUpper(code)]; isIndex {
		return h.fetchLegacyUSStockKLine(ctx, code, period, limit)
	}
	primaryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	rows, err := h.fetchBitgetKLine(primaryCtx, "us_stock", code, period, limit)
	cancel()
	if err == nil && len(rows) > 0 {
		return rows, nil
	}
	// The old Sina endpoint only supplies daily bars. Never label those as
	// intraday bars; weekly requests aggregate them into real calendar weeks.
	if period != "1d" && period != "1w" {
		return nil, err
	}
	n := limit
	if period == "1w" {
		n = limit * 7
	}
	rows, err = h.fetchLegacyUSStockKLine(ctx, code, "1d", n)
	if period == "1w" && err == nil {
		var days []bitget.Candle
		for _, row := range rows {
			date, e := time.Parse("2006-01-02", row.Date)
			if e == nil {
				days = append(days, bitget.Candle{Time: date, Open: row.Open, Close: row.Close, High: row.High, Low: row.Low, Volume: row.Volume})
			}
		}
		rows = nil
		for _, c := range bitget.Aggregate(days, "1w") {
			rows = append(rows, KLineResponse{Date: c.Time.Format("2006-01-02"), Open: c.Open, Close: c.Close, High: c.High, Low: c.Low, Volume: c.Volume})
		}
		if len(rows) > limit {
			rows = rows[len(rows)-limit:]
		}
	}
	for i := range rows {
		rows[i].Source = "sina"
	}
	return rows, err
}

func (h *StockHandler) fetchHKStockKLine(ctx context.Context, code, period string, limit int) ([]KLineResponse, error) {
	primaryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	rows, err := h.fetchBitgetKLine(primaryCtx, "hk_stock", code, period, limit)
	cancel()
	if err == nil && len(rows) > 0 {
		return rows, nil
	}
	if period != "1d" && period != "1w" {
		return nil, err
	}
	rows, err = h.fetchTencentKLine(ctx, "hk"+bitget.NormalizeCode("hk_stock", code), period, limit)
	for i := range rows {
		rows[i].Source = "tencent"
	}
	return rows, err
}

func mergeWatchlistQuotes(items []model.WatchlistItem, quotes []StockResponse) []StockResponse {
	byCode := make(map[string]StockResponse)
	for _, q := range quotes {
		byCode[q.Market+":"+canonicalWatchCode(q.Market, q.ID, q.Symbol)] = q
	}
	rows := make([]StockResponse, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		code := canonicalWatchCode(item.Market, item.StockCode, item.Symbol)
		key := item.Market + ":" + code
		if seen[key] {
			continue
		}
		seen[key] = true
		q, ok := byCode[key]
		// Older A-share records used bare codes; prefer their stored exchange.
		if !ok && item.Market == "a_share" {
			prefix := "sh"
			if strings.HasPrefix(strings.ToLower(item.Symbol), "sz") || (item.Symbol == "" && (strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3"))) {
				prefix = "sz"
			}
			q, ok = byCode[item.Market+":"+prefix+code]
		}
		if !ok {
			available := false
			q = StockResponse{ID: code, Symbol: item.Symbol, Name: item.Name, Market: item.Market, QuoteAvailable: &available}
		}
		if item.Name != "" {
			q.Name = item.Name
		}
		rows = append(rows, q)
	}
	return rows
}

func canonicalWatchCode(market, code, symbol string) string {
	code = strings.TrimSpace(code)
	if market == "us_stock" || market == "hk_stock" {
		return bitget.NormalizeCode(market, code)
	}
	if market == "crypto" {
		if mapped, ok := cryptoIndexToSymbol[strings.ToLower(code)]; ok {
			return mapped
		}
		return strings.TrimSuffix(strings.TrimSuffix(strings.ToUpper(code), "USDT"), "/")
	}
	if market == "a_share" {
		code = strings.ToLower(code)
		if len(code) == 6 {
			prefix := "sh"
			if strings.HasPrefix(strings.ToLower(symbol), "sz") || (symbol == "" && (strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3"))) {
				prefix = "sz"
			}
			return prefix + code
		}
	}
	return code
}

// GetAllWatchlist reads the existing per-market records, scoped to the same
// authenticated user as iOS. Persisted entries survive quote/provider failures.
func (h *StockHandler) GetAllWatchlist(c *gin.Context) {
	rows, err := h.readWatchlist(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get watchlist"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *StockHandler) readWatchlist(parent context.Context, user uint) ([]StockResponse, error) {
	items, err := h.watchlistRepo.GetByUserContext(parent, user)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	var quotes []StockResponse
	for _, market := range []string{"a_share", "us_stock", "hk_stock", "crypto"} {
		codes := make([]string, 0)
		for _, item := range items {
			if item.Market == market {
				code := item.StockCode
				if market == "a_share" && len(code) == 6 {
					prefix := "sh"
					sym := strings.ToLower(item.Symbol)
					if strings.HasPrefix(sym, "sz") || (sym == "" && (strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3"))) {
						prefix = "sz"
					}
					code = prefix + code
				}
				codes = append(codes, code)
			}
		}
		if len(codes) == 0 {
			continue
		}
		wg.Add(1)
		go func(market string, codes []string) {
			defer wg.Done()
			var rows []StockResponse
			switch market {
			case "a_share":
				rows, _ = h.fetchAShareStocksByCode(ctx, strings.Join(codes, ","))
			case "us_stock":
				rows, _ = h.fetchUSStocksBySymbol(ctx, strings.Join(codes, ","))
			case "hk_stock":
				rows, _ = h.fetchHKStocksByCode(ctx, codes)
			case "crypto":
				var symbols []string
				for _, code := range codes {
					if mapped, ok := cryptoIndexToSymbol[strings.ToLower(code)]; ok {
						code = mapped
					}
					if !strings.HasSuffix(strings.ToUpper(code), "USDT") {
						code += "USDT"
					}
					symbols = append(symbols, strings.ToUpper(code))
				}
				rows, _ = h.fetchBitgetCryptoStocks(ctx, symbols)
			}
			mu.Lock()
			quotes = append(quotes, rows...)
			mu.Unlock()
		}(market, codes)
	}
	wg.Wait()
	return mergeWatchlistQuotes(items, quotes), nil
}

func (h *StockHandler) productRequest(c *gin.Context, candles bool) bool {
	symbol, category := c.Query("provider_symbol"), c.Query("category")
	if symbol == "" && category == "" {
		return false
	}
	if symbol == "" || len(symbol) > 60 || (category != "SPOT" && category != "USDT-FUTURES") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product identity"})
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if !candles {
		q, err := h.bitget.ProductQuote(ctx, category, symbol)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "该产品行情暂不可用"})
		} else {
			c.JSON(http.StatusOK, bitgetStock(q))
		}
		return true
	}
	period := c.DefaultQuery("period", "1d")
	if !map[string]bool{"5m": true, "15m": true, "30m": true, "1h": true, "4h": true, "1d": true, "1w": true}[period] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period"})
		return true
	}
	limit := int(parseFloat(c.DefaultQuery("limit", "150")))
	rows, err := h.bitget.ProductCandles(ctx, category, symbol, c.DefaultQuery("period", "1d"), limit)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "该产品 K 线暂不可用"})
		return true
	}
	result := make([]KLineResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, KLineResponse{Date: candleDate(row.Time, c.DefaultQuery("period", "1d")), Open: row.Open, Close: row.Close, High: row.High, Low: row.Low, Volume: row.Volume, Source: "bitget"})
	}
	c.JSON(http.StatusOK, result)
	return true
}

func candleDate(stamp time.Time, period string) string {
	if period == "1d" || period == "1w" {
		return stamp.Format("2006-01-02")
	}
	return stamp.Format(time.RFC3339)
}
