package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// FMPQuoteResponse represents the response from FMP API's real-time quote endpoint
type FMPQuoteResponse struct {
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	Price             float64 `json:"price"`
	ChangePercentage  float64 `json:"changePercentage"`
	Change            float64 `json:"change"`
	Volume            int64   `json:"volume"`
	DayLow            float64 `json:"dayLow"`
	DayHigh           float64 `json:"dayHigh"`
	YearHigh          float64 `json:"yearHigh"`
	YearLow           float64 `json:"yearLow"`
	MarketCap         int64   `json:"marketCap"`
	PriceAvg50        float64 `json:"priceAvg50"`
	PriceAvg200       float64 `json:"priceAvg200"`
	Exchange          string  `json:"exchange"`
	Open              float64 `json:"open"`
	PreviousClose     float64 `json:"previousClose"`
	Timestamp         int64   `json:"timestamp"`
}

// FMPAftermarketTradeResponse represents aftermarket (pre-market/after-hours) trade data
type FMPAftermarketTradeResponse struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	TradeSize int     `json:"tradeSize"`
	Timestamp int64   `json:"timestamp"`
}

// FMPAftermarketQuoteResponse represents aftermarket (pre-market/after-hours) quote data
type FMPAftermarketQuoteResponse struct {
	Symbol    string  `json:"symbol"`
	BidSize   int     `json:"bidSize"`
	BidPrice  float64 `json:"bidPrice"`
	AskSize   int     `json:"askSize"`
	AskPrice  float64 `json:"askPrice"`
	Volume    int64   `json:"volume"`
	Timestamp int64   `json:"timestamp"`
}

// fetchFMPQuote fetches real-time quote from FMP API (supports concurrent requests)
// This API provides the most current price available, including extended hours data
func (h *StockHandler) fetchFMPQuote(ctx context.Context, symbol string) (*FMPQuoteResponse, error) {
	if h.fmpAPIKey == "" {
		return nil, fmt.Errorf("FMP API key not configured")
	}

	url := fmt.Sprintf("https://financialmodelingprep.com/stable/quote?symbol=%s&apikey=%s", 
		strings.ToUpper(symbol), h.fmpAPIKey)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("FMP API returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var quotes []FMPQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&quotes); err != nil {
		return nil, fmt.Errorf("failed to parse FMP response: %w", err)
	}
	
	if len(quotes) == 0 {
		return nil, fmt.Errorf("no data returned from FMP for %s", symbol)
	}
	
	return &quotes[0], nil
}

// fetchFMPAftermarketTrade fetches real-time extended hours trade data from FMP API
// This API provides the actual pre-market/after-hours trading price
func (h *StockHandler) fetchFMPAftermarketTrade(ctx context.Context, symbol string) (*FMPAftermarketTradeResponse, error) {
	if h.fmpAPIKey == "" {
		return nil, fmt.Errorf("FMP API key not configured")
	}

	url := fmt.Sprintf("https://financialmodelingprep.com/stable/aftermarket-trade?symbol=%s&apikey=%s",
		strings.ToUpper(symbol), h.fmpAPIKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("FMP API returned status %d: %s", resp.StatusCode, string(body))
	}

	var trades []FMPAftermarketTradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&trades); err != nil {
		return nil, fmt.Errorf("failed to parse FMP response: %w", err)
	}

	if len(trades) == 0 {
		return nil, fmt.Errorf("no aftermarket trade data returned from FMP for %s", symbol)
	}

	return &trades[0], nil
}

// fetchFMPQuotesConcurrent fetches quotes for multiple symbols concurrently from FMP API
// Returns a map of symbol -> FMPQuoteResponse
func (h *StockHandler) fetchFMPQuotesConcurrent(ctx context.Context, symbols []string) map[string]*FMPQuoteResponse {
	results := make(map[string]*FMPQuoteResponse)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrency to avoid overwhelming the API
	semaphore := make(chan struct{}, 5) // Max 5 concurrent requests

	for _, symbol := range symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			quote, err := h.fetchFMPQuote(ctx, sym)
			if err != nil {
				h.logger.Error("Failed to fetch FMP quote for %s: %v", sym, err)
				return
			}

			mu.Lock()
			results[strings.ToUpper(sym)] = quote
			mu.Unlock()
		}(symbol)
	}

	wg.Wait()
	return results
}

// fetchFMPAftermarketTradesConcurrent fetches aftermarket trades for multiple symbols concurrently
// Returns a map of symbol -> FMPAftermarketTradeResponse with real-time extended hours prices
func (h *StockHandler) fetchFMPAftermarketTradesConcurrent(ctx context.Context, symbols []string) map[string]*FMPAftermarketTradeResponse {
	results := make(map[string]*FMPAftermarketTradeResponse)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrency to avoid overwhelming the API
	semaphore := make(chan struct{}, 5) // Max 5 concurrent requests

	for _, symbol := range symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			trade, err := h.fetchFMPAftermarketTrade(ctx, sym)
			if err != nil {
				// It's normal to have no aftermarket data during regular hours
				// So we don't log this as an error
				return
			}

			mu.Lock()
			results[strings.ToUpper(sym)] = trade
			mu.Unlock()
		}(symbol)
	}

	wg.Wait()
	return results
}
