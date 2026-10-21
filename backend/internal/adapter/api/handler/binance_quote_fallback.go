package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func binanceQuoteTime(millis int64) string {
	if millis <= 0 {
		return ""
	}
	return time.UnixMilli(millis).UTC().Format(time.RFC3339)
}
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
			Source:        "binance", Currency: "USDT", ProductType: "crypto_spot", ChangeBasis: "24h", QuoteTime: binanceQuoteTime(t.CloseTime),
		})
	}
	return stocks, nil
}

func (h *StockHandler) fetchBinanceCryptoKLine(ctx context.Context, coinID string, period string, limit int) ([]KLineResponse, error) {
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
	for i := range klines {
		klines[i].Source = "binance"
	}
	return klines, nil
}
