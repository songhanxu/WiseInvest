package bitget

import (
	"context"
	"fmt"
	"strings"
)

// Crypto uses the same public spot catalogue and ticker cache as stocks.
// Validate the instrument metadata so stock symbols cannot enter the crypto list.
func (c *Client) CryptoQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	instruments, err := c.AccountInventory(ctx, "SPOT")
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(symbols))
	for _, symbol := range symbols {
		wanted[cryptoSymbol(symbol)] = true
	}
	tickers, err := c.tickers(ctx, "us_stock")
	if err != nil {
		return nil, err
	}
	rows := make([]Quote, 0, len(symbols))
	for _, item := range instruments {
		if !wanted[item.Symbol] || item.SymbolType != "crypto" || item.QuoteCoin != "USDT" || item.Status != "online" {
			continue
		}
		ticker, ok := tickers[item.Symbol]
		if !ok || number(ticker.Last) <= 0 {
			continue
		}
		q := ApplyTicker(HoldingIdentity(item, "SPOT"), ticker)
		rows = append(rows, q)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Bitget crypto quotes unavailable")
	}
	// Match the caller's order (the upstream catalogue has no stable ordering).
	ordered := make([]Quote, 0, len(rows))
	bySymbol := make(map[string]Quote, len(rows))
	for _, row := range rows {
		bySymbol[row.ProviderSymbol] = row
	}
	for _, symbol := range symbols {
		if row, ok := bySymbol[cryptoSymbol(symbol)]; ok {
			ordered = append(ordered, row)
		}
	}
	return ordered, nil
}

func cryptoSymbol(code string) string {
	code = strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), "/", "")
	if !strings.HasSuffix(code, "USDT") {
		code += "USDT"
	}
	return code
}

func (c *Client) CryptoCandles(ctx context.Context, code, period string, limit int) ([]Candle, error) {
	symbol := cryptoSymbol(code)
	items, err := c.AccountInventory(ctx, "SPOT")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Symbol == symbol && item.SymbolType == "crypto" && item.QuoteCoin == "USDT" && item.Status == "online" {
			return c.candlesForSymbol(ctx, "SPOT", symbol, period, limit)
		}
	}
	return nil, fmt.Errorf("crypto is not listed on Bitget")
}
