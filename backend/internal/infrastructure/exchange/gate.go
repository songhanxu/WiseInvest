package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Gate can encode contract counts as JSON numbers or strings.
type amount string

type gatePair struct {
	ID, Base, Quote string
	Name            string `json:"base_name"`
}

func (a *amount) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else {
		s = string(b)
	}
	*a = amount(s)
	return nil
}
func (c *Client) gate(ctx context.Context) (Result, error) {
	r := Result{Holdings: []Holding{}, Warnings: []string{}}
	success := 0
	byCoin := map[string]gatePair{}
	loadPairs := func() {
		var pairs []gatePair
		warn(&r, "现货产品资料", c.get(ctx, "/spot/currency_pairs", nil, &pairs))
		for _, pair := range pairs {
			if pair.Quote == "USDT" {
				byCoin[pair.Base] = pair
			}
		}
	}
	if c.cfg.AccountType == "unified" {
		var data struct {
			Balances *map[string]struct{ Equity string } `json:"balances"`
		}
		err := c.get(ctx, "/unified/accounts", nil, &data)
		if err == nil && data.Balances == nil {
			err = fmt.Errorf("余额列表缺失")
		}
		if err != nil {
			warn(&r, "统一账户", err)
		} else {
			success++
			for coin, a := range *data.Balances {
				if nonzero(a.Equity) {
					h := baseHolding(coin, a.Equity)
					h.Type = "balance"
					r.Holdings = append(r.Holdings, h)
				}
			}
		}
	} else {
		var data []struct{ Currency, Available, Locked string }
		err := c.get(ctx, "/spot/accounts", nil, &data)
		if err != nil {
			warn(&r, "现货账户", err)
		} else {
			success++
			for _, a := range data {
				q := decimalSum(a.Available, a.Locked)
				if nonzero(q) {
					r.Holdings = append(r.Holdings, baseHolding(a.Currency, q))
				}
			}
		}
	}
	if len(r.Holdings) > 0 {
		loadPairs()
		var tickers []struct {
			Symbol string `json:"currency_pair"`
			Last   string
		}
		warn(&r, "现货行情", c.get(ctx, "/spot/tickers", nil, &tickers))
		prices := map[string]string{}
		for _, t := range tickers {
			prices[t.Symbol] = t.Last
		}
		for i := range r.Holdings {
			h := &r.Holdings[i]
			pair := byCoin[h.Quote.Code]
			h.Quote.ProviderSymbol = pair.ID
			if pair.Name != "" {
				h.Quote.Name = pair.Name
			}
			if strings.Contains(strings.ToLower(pair.Name), "xstock") {
				h.Quote.Market = "other"
				h.Quote.ProductType = "stock_token"
			}
			if h.Quote.Code == "USDT" {
				finishSpot(h, "1")
			} else {
				finishSpot(h, prices[pair.ID])
			}
		}
	}
	if c.cfg.IncludeStocks {
		var data struct {
			Data *struct {
				List *[]struct {
					Symbol, Exchange string
					Name             string `json:"symbol_desc"`
					Currency         string `json:"quote_currency"`
					Volume           string `json:"volume"`
					Price            string `json:"last_price"`
					Cost             string `json:"avg_cost_price"`
					PnL              string `json:"position_pnl"`
				} `json:"list"`
			} `json:"data"`
		}
		err := c.get(ctx, "/stock/positions", nil, &data)
		if err == nil && (data.Data == nil || data.Data.List == nil) {
			err = fmt.Errorf("股票持仓列表缺失")
		}
		if err != nil {
			warn(&r, "股票账户", err)
		} else {
			success++
			for _, p := range *data.Data.List {
				if !nonzero(p.Volume) {
					continue
				}
				h := baseHolding(p.Symbol, p.Volume)
				h.ID = "equity:" + p.Exchange + ":" + p.Symbol
				h.Quote.Market = "other"
				switch p.Exchange {
				case "us":
					h.Quote.Market = "us_stock"
				case "hk":
					h.Quote.Market = "hk_stock"
				}
				h.Quote.Name = p.Name
				h.Quote.Currency = p.Currency
				h.Quote.ProductType = "equity"
				h.Quote.ProviderSymbol = p.Symbol
				h.EntryPrice = p.Cost
				h.PnL = p.PnL
				h.MarginCurrency = p.Currency
				h.Unit = "股"
				finishSpot(&h, p.Price)
				r.Holdings = append(r.Holdings, h)
			}
		}
	}
	if c.cfg.IncludeFutures {
		var positions []struct {
			Contract   string
			Size       amount
			Mode       string
			Entry      string `json:"entry_price"`
			Mark       string `json:"mark_price"`
			PnL        string `json:"unrealised_pnl"`
			MarginMode string `json:"pos_margin_mode"`
		}
		err := c.get(ctx, "/futures/usdt/positions", url.Values{"holding": {"true"}}, &positions)
		if err != nil {
			warn(&r, "USDT 合约", err)
		} else {
			success++
			if len(positions) > 0 && len(byCoin) == 0 {
				loadPairs()
			}
			for _, p := range positions {
				q := string(p.Size)
				if !nonzero(q) {
					continue
				}
				h := baseHolding(p.Contract, abs(q))
				h.ID = "futures:" + p.Contract + ":" + p.Mode
				h.Type = "futures"
				h.Unit = "张"
				h.Side = "long"
				if strings.HasPrefix(q, "-") || p.Mode == "dual_short" {
					h.Side = "short"
				}
				h.EntryPrice = p.Entry
				h.PnL = p.PnL
				h.MarginCurrency = "USDT"
				h.MarginMode = p.MarginMode
				h.Quote.ProviderSymbol = p.Contract
				h.Quote.Price = num(p.Mark)
				h.Quote.ProductType = "other_perpetual"
				h.Quote.Market = "other"
				base := strings.TrimSuffix(p.Contract, "_USDT")
				if pair, ok := byCoin[base]; ok && !strings.Contains(strings.ToLower(pair.Name), "xstock") {
					h.Quote.Code = base
					h.Quote.Name = pair.Name
					h.Quote.Market = "crypto"
					h.Quote.ProductType = "crypto_perpetual"
				}
				r.Holdings = append(r.Holdings, h)
			}
		}
	}
	return finish(r, success)
}
