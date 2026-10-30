package handler

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
)

func (h *StockHandler) marketQuoteLock(market string) *sync.Mutex {
	value, _ := h.quoteLocks.LoadOrStore(market, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func quoteFreshTTL(market string, row StockResponse) time.Duration {
	if market == "crypto" {
		return 15 * time.Second
	}
	if row.QuoteSession == "close" {
		return 15 * time.Minute
	}
	if stamp, err := time.Parse(time.RFC3339, row.QuoteTime); err == nil && time.Since(stamp) > 2*time.Hour {
		return 5 * time.Minute
	}
	return 30 * time.Second
}

func lastQuoteKey(market, code string) string {
	return "quote:last:" + market + ":" + code
}

func (h *StockHandler) fetchQuoteBatch(ctx context.Context, market string, codes []string) ([]StockResponse, error) {
	switch market {
	case "a_share":
		return h.fetchAShareStocksByCode(ctx, strings.Join(codes, ","))
	case "us_stock":
		return h.fetchUSStocksBySymbol(ctx, strings.Join(codes, ","))
	case "hk_stock":
		return h.fetchHKStocksByCode(ctx, codes)
	case "crypto":
		symbols := make([]string, len(codes))
		for i, code := range codes {
			symbols[i] = code + "USDT"
		}
		return h.fetchBitgetCryptoStocks(ctx, symbols)
	}
	return nil, nil
}

func (h *StockHandler) cacheFreshQuotes(ctx context.Context, market string, rows []StockResponse) {
	if h.cache == nil {
		return
	}
	for _, row := range rows {
		if row.CurrentPrice <= 0 || !quoteCacheSourceAllowed(market, row.Source) {
			continue
		}
		code := canonicalWatchCode(market, row.ID, row.Symbol)
		row.QuoteStale = false
		_ = h.cache.SetJSON(ctx, cache.StockQuoteKey(market, code), row, quoteFreshTTL(market, row))
		_ = h.cache.SetJSON(ctx, lastQuoteKey(market, code), row, 7*24*time.Hour)
	}
}

// watchlistQuotes shares one short-lived Redis quote per symbol across users and
// endpoints. On expiry, a last-known quote is returned with an explicit stale
// flag while one background fetch refreshes the cache.
func (h *StockHandler) watchlistQuotes(ctx context.Context, market string, requested []string) ([]StockResponse, error) {
	lock := h.marketQuoteLock(market)
	lock.Lock()
	defer lock.Unlock()

	unique := make([]string, 0, len(requested))
	seen := make(map[string]bool)
	for _, raw := range requested {
		code := canonicalWatchCode(market, raw, raw)
		if market == "crypto" && code == "FEAR_GREED" {
			continue
		}
		if code != "" && !seen[code] {
			unique = append(unique, code)
			seen[code] = true
		}
	}
	rows := make(map[string]StockResponse, len(unique))
	stale := make(map[string]StockResponse, len(unique))
	missing := make([]string, 0, len(unique))
	for _, code := range unique {
		var fresh StockResponse
		if h.cache != nil {
			hit, _ := h.cache.GetJSON(ctx, cache.StockQuoteKey(market, code), &fresh)
			if hit && fresh.CurrentPrice > 0 && quoteCacheSourceAllowed(market, fresh.Source) {
				rows[code] = fresh
				continue
			}
			var previous StockResponse
			hit, _ = h.cache.GetJSON(ctx, lastQuoteKey(market, code), &previous)
			if hit && previous.CurrentPrice > 0 && quoteCacheSourceAllowed(market, previous.Source) {
				previous.QuoteStale = true
				stale[code] = previous
			}
		}
		missing = append(missing, code)
	}
	if len(missing) > 0 && len(stale) == len(missing) && h.cache != nil {
		batchKey := market + ":" + strings.Join(missing, ",")
		if _, active := h.quoteRefreshing.LoadOrStore(batchKey, true); !active {
			batch := append([]string(nil), missing...)
			go func() {
				defer h.quoteRefreshing.Delete(batchKey)
				refreshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				fresh, _ := h.fetchQuoteBatch(refreshCtx, market, batch)
				h.cacheFreshQuotes(refreshCtx, market, fresh)
			}()
		}
		for code, row := range stale {
			rows[code] = row
		}
	} else if len(missing) > 0 {
		fresh, err := h.fetchQuoteBatch(ctx, market, missing)
		h.cacheFreshQuotes(ctx, market, fresh)
		for _, row := range fresh {
			if row.CurrentPrice <= 0 || !quoteCacheSourceAllowed(market, row.Source) {
				continue
			}
			code := canonicalWatchCode(market, row.ID, row.Symbol)
			rows[code] = row
		}
		for code, previous := range stale {
			if _, ok := rows[code]; !ok {
				rows[code] = previous
			}
		}
		if err != nil && len(rows) == 0 {
			return nil, err
		}
	}
	result := make([]StockResponse, 0, len(rows))
	for _, code := range unique {
		if row, ok := rows[code]; ok {
			result = append(result, row)
		}
	}
	return result, nil
}
