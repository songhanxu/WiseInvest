package exchange

import (
	"context"
	"fmt"
	"strings"
)

type binanceInstrument struct {
	Symbol         string
	BaseAsset      string
	QuoteAsset     string
	UnderlyingType string
}

func (c *Client) binance(ctx context.Context) (Result, error) {
	r := Result{Holdings: []Holding{}, Warnings: []string{}}
	success := 0
	var account struct {
		Balances *[]struct{ Asset, Free, Locked string } `json:"balances"`
	}
	err := c.get(ctx, "/api/v3/account", nil, &account)
	if err == nil && account.Balances == nil {
		err = fmt.Errorf("余额列表缺失")
	}
	if err != nil {
		warn(&r, "现货账户", err)
	} else {
		success++
		for _, a := range *account.Balances {
			q := decimalSum(a.Free, a.Locked)
			if nonzero(q) {
				r.Holdings = append(r.Holdings, baseHolding(a.Asset, q))
			}
		}
	}
	if len(r.Holdings) > 0 {
		var instruments struct{ Symbols []binanceInstrument }
		warn(&r, "现货产品资料", c.get(ctx, "/api/v3/exchangeInfo", nil, &instruments))
		pairs := map[string]string{}
		for _, i := range instruments.Symbols {
			if i.QuoteAsset == "USDT" {
				pairs[i.BaseAsset] = i.Symbol
			}
		}
		var tickers []struct{ Symbol, Price string }
		warn(&r, "现货行情", c.get(ctx, "/api/v3/ticker/price", nil, &tickers))
		prices := map[string]string{}
		for _, t := range tickers {
			prices[t.Symbol] = t.Price
		}
		for i := range r.Holdings {
			h := &r.Holdings[i]
			h.Quote.ProviderSymbol = pairs[h.Quote.Code]
			if h.Quote.Code == "USDT" {
				finishSpot(h, "1")
			} else {
				finishSpot(h, prices[h.Quote.ProviderSymbol])
			}
		}
		if c.cfg.IncludeStocks {
			var tokens []struct{ AssetCode, AssetName, UnderlyingEquitySymbol string }
			err := c.get(ctx, "/sapi/v1/equity/market/tokenized-assets", nil, &tokens)
			warn(&r, "股票代币识别", err)
			for i := range r.Holdings {
				for _, t := range tokens {
					if r.Holdings[i].Unit == t.AssetCode && t.UnderlyingEquitySymbol != "" {
						r.Holdings[i].Quote.Code = t.UnderlyingEquitySymbol
						r.Holdings[i].Quote.Name = t.AssetName
						r.Holdings[i].Quote.Market = "us_stock"
						r.Holdings[i].Quote.ProductType = "stock_token"
					}
				}
			}
		}
	}
	if c.cfg.IncludeFutures {
		var positions []struct{ Symbol, PositionSide, PositionAmt, EntryPrice, MarkPrice, UnRealizedProfit, MarginAsset string }
		err := c.get(ctx, "/fapi/v3/positionRisk", nil, &positions)
		if err != nil {
			warn(&r, "USDⓈ-M 合约", err)
		} else {
			success++
			var info struct{ Symbols []binanceInstrument }
			meta := map[string]binanceInstrument{}
			active := false
			for _, p := range positions {
				if nonzero(p.PositionAmt) {
					active = true
				}
			}
			if active {
				warn(&r, "合约产品资料", c.get(ctx, "/fapi/v1/exchangeInfo", nil, &info))
				for _, i := range info.Symbols {
					meta[i.Symbol] = i
				}
			}
			for _, p := range positions {
				if !nonzero(p.PositionAmt) {
					continue
				}
				i, known := meta[p.Symbol]
				h := baseHolding(i.BaseAsset, abs(p.PositionAmt))
				h.Type = "futures"
				h.ID = "futures:" + p.Symbol + ":" + p.PositionSide
				h.Side = "long"
				if strings.HasPrefix(p.PositionAmt, "-") || p.PositionSide == "SHORT" {
					h.Side = "short"
				}
				h.EntryPrice = p.EntryPrice
				h.PnL = p.UnRealizedProfit
				h.MarginCurrency = p.MarginAsset
				h.Quote.ProviderSymbol = p.Symbol
				h.Quote.Currency = i.QuoteAsset
				h.Quote.Price = num(p.MarkPrice)
				h.Quote.ProductType = "crypto_perpetual"
				if !known {
					h.Quote.Code = p.Symbol
					h.Quote.Name = p.Symbol
					h.Quote.Market = "other"
					h.Quote.Currency = p.MarginAsset
					h.Unit = "基础资产"
				}
				switch i.UnderlyingType {
				case "EQUITY":
					h.Quote.Market = "us_stock"
					h.Quote.ProductType = "stock_perpetual"
				case "HK_EQUITY":
					h.Quote.Market = "other"
					h.Quote.ProductType = "stock_perpetual"
				case "COIN", "":
				default:
					h.Quote.Market = "other"
					h.Quote.ProductType = "other_perpetual"
				}
				r.Holdings = append(r.Holdings, h)
			}
		}
	}
	return finish(r, success)
}
