package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/exchange"
)

type AccountSyncState struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	AsOf    string `json:"as_of,omitempty"`
}

func exchangeSummary(cfg config.ExchangeAccountConfig, user uint) AccountSummary {
	name := "Gate"
	if cfg.Provider == "binance" {
		name = "币安"
	}
	a := AccountSummary{ID: cfg.Provider, Provider: cfg.Provider, Name: name, Status: "not_configured", Message: "账户尚未连接"}
	if !cfg.HasCredentials() || cfg.OwnerUserID == 0 {
		return a
	}
	if user != cfg.OwnerUserID {
		a.Status = "unbound"
		a.Message = "当前慧投用户未绑定此账户"
		return a
	}
	a.Name = cfg.AccountName
	a.AccountType = cfg.AccountType
	a.Status = "ready"
	a.Message = "只读同步现货余额"
	if cfg.AccountType == "unified" {
		a.Message = "统一账户显示币种权益，不能与合约名义价值直接相加"
	}
	return a
}
func (h *AccountHandler) AddExchange(cfg config.ExchangeAccountConfig) {
	if h.others == nil {
		h.others = map[string]*AccountHandler{}
	}
	client := exchange.NewClient(cfg)
	h.others[cfg.Provider] = &AccountHandler{exchangeConfig: &cfg, loader: func(ctx context.Context) (HoldingsSnapshot, error) {
		r, err := client.Read(ctx)
		if err != nil {
			return HoldingsSnapshot{}, err
		}
		data := HoldingsSnapshot{AccountID: cfg.Provider, Holdings: []AccountHolding{}, Warnings: r.Warnings, Complete: len(r.Warnings) == 0, AsOf: time.Now().UTC().Format(time.RFC3339)}
		for _, row := range r.Holdings {
			stock := bitgetStock(row.Quote)
			stock.Source = cfg.Provider
			stock.ChangeBasis = ""
			available := row.Quote.Price > 0
			stock.QuoteAvailable = &available
			data.Holdings = append(data.Holdings, AccountHolding{StockResponse: stock, HoldingID: row.ID, HoldingType: row.Type, Side: row.Side, Quantity: row.Quantity, QuantityUnit: row.Unit, EntryPrice: row.EntryPrice, UnrealizedPnL: row.PnL, MarginCurrency: row.MarginCurrency, MarginMode: row.MarginMode, MarketValue: row.MarketValue})
		}
		return data, nil
	}}
}
func (h *AccountHandler) summaries(user uint) []AccountSummary {
	rows := []AccountSummary{h.summary(user)}
	for _, id := range []string{"binance", "gate"} {
		if account := h.others[id]; account != nil {
			rows = append(rows, account.summary(user))
		}
	}
	return rows
}
func decorateSnapshot(data HoldingsSnapshot, state AccountSummary) HoldingsSnapshot {
	data.AccountID = state.ID
	rows := append([]AccountHolding{}, data.Holdings...)
	for i := range rows {
		rows[i].AccountID = state.ID
		rows[i].AccountName = state.Name
		rows[i].HoldingID = state.ID + ":" + rows[i].HoldingID
		rows[i].SnapshotTime = data.AsOf
		rows[i].Stale = data.Stale
	}
	data.Holdings = rows
	return data
}
func (h *AccountHandler) allHoldings(ctx context.Context, user uint) (HoldingsSnapshot, error) {
	summaries := h.summaries(user)
	data := HoldingsSnapshot{AccountID: "all", Holdings: []AccountHolding{}, Warnings: []string{}, Complete: true}
	type outcome struct {
		snapshot  HoldingsSnapshot
		err       error
		attempted bool
	}
	out := make([]outcome, len(summaries))
	var wg sync.WaitGroup
	for i, state := range summaries {
		if state.Status != "ready" {
			continue
		}
		target := h
		if state.ID != "bitget" {
			target = h.others[state.ID]
		}
		wg.Add(1)
		go func(i int, target *AccountHandler) {
			defer wg.Done()
			out[i].attempted = true
			out[i].snapshot, out[i].err = target.holdings(ctx)
		}(i, target)
	}
	wg.Wait()
	attempted, success := 0, 0
	for i, state := range summaries {
		status := AccountSyncState{ID: state.ID, Name: state.Name, Status: state.Status, Message: state.Message}
		o := out[i]
		if !o.attempted {
			data.Accounts = append(data.Accounts, status)
			continue
		}
		attempted++
		if o.err != nil {
			data.Complete = false
			status.Status = "failed"
			status.Message = o.err.Error()
			data.Warnings = append(data.Warnings, state.Name+"："+o.err.Error())
		} else {
			success++
			snapshot := decorateSnapshot(o.snapshot, state)
			status.AsOf = snapshot.AsOf
			status.Status = "synced"
			status.Message = ""
			if snapshot.Stale {
				status.Status = "stale"
				data.Stale = true
			} else if !snapshot.Complete {
				status.Status = "partial"
			}
			if !snapshot.Complete {
				data.Complete = false
			}
			for _, w := range snapshot.Warnings {
				data.Warnings = append(data.Warnings, state.Name+"："+w)
			}
			data.Holdings = append(data.Holdings, snapshot.Holdings...)
			if data.AsOf == "" || snapshot.AsOf < data.AsOf {
				data.AsOf = snapshot.AsOf
			}
		}
		data.Accounts = append(data.Accounts, status)
	}
	if attempted > 0 && success == 0 {
		return data, fmt.Errorf("已连接账户均同步失败：%s", strings.Join(data.Warnings, "；"))
	}
	sort.SliceStable(data.Holdings, func(i, j int) bool {
		a, b := data.Holdings[i], data.Holdings[j]
		if a.Market != b.Market {
			return a.Market < b.Market
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.HoldingID < b.HoldingID
	})
	return data, nil
}
