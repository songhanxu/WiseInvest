package handler

import (
	"context"
	"strings"
	"sync"
)

func bitgetBoardCacheAllowed(market string, rows []IndexResponse) bool {
	for _, row := range rows {
		if !quoteCacheSourceAllowed(market, row.Source) {
			return false
		}
	}
	return true
}

func quoteCacheSourceAllowed(market, source string) bool {
	return market == "a_share" || source == "bitget" || ((market == "us_stock" || market == "hk_stock") && source == "tencent") || (market == "crypto" && source == "binance")
}

func (h *StockHandler) fetchBitgetMarketBoard(ctx context.Context, market string, codes []string) ([]IndexResponse, error) {
	var quotes []StockResponse
	var err error
	if market == "us_stock" {
		quotes, err = h.fetchUSStocksBySymbol(ctx, strings.Join(codes, ","))
	} else {
		quotes, err = h.fetchHKStocksByCode(ctx, codes)
	}
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]StockResponse, len(quotes))
	for _, quote := range quotes {
		byCode[quote.ID] = quote
	}
	rows := make([]IndexResponse, 0, len(quotes))
	for _, code := range codes {
		if quote, ok := byCode[code]; ok {
			rows = append(rows, IndexResponse{ID: quote.ID, Name: quote.Name, ShortName: quote.Symbol, Value: quote.CurrentPrice, Change: quote.Change, ChangePercent: quote.ChangePercent, Source: quote.Source, Currency: quote.Currency, ProductType: quote.ProductType, QuoteTime: quote.QuoteTime, QuoteSession: quote.QuoteSession})
		}
	}
	// Fetch actual closing prices for chart previews. An upstream failure leaves
	// the chart empty, rather than fabricating a trend from the current quote.
	var wg sync.WaitGroup
	for i := range rows {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if rows[i].Source != "bitget" {
				code := "us." + rows[i].ID
				if market == "hk_stock" {
					code = "hk" + rows[i].ID
				}
				candles, err := h.fetchTencentKLine(ctx, code, "1d", 24)
				if err == nil {
					for _, candle := range candles {
						rows[i].SparklineData = append(rows[i].SparklineData, candle.Close)
					}
				}
				return
			}
			candles, err := h.bitget.Candles(ctx, market, rows[i].ID, "1h", 24)
			if err == nil {
				for _, candle := range candles {
					rows[i].SparklineData = append(rows[i].SparklineData, candle.Close)
				}
			}
		}(i)
	}
	wg.Wait()
	return rows, nil
}
