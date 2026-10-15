package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

type accountReader interface {
	Assets(context.Context, string) ([]bitget.Asset, error)
	Positions(context.Context, string) ([]bitget.Position, error)
}

type AccountSummary struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	Name        string `json:"name"`
	AccountType string `json:"account_type"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

type AccountHolding struct {
	EntryPrice     string `json:"entry_price,omitempty"`
	UnrealizedPnL  string `json:"unrealized_pnl,omitempty"`
	MarginCurrency string `json:"margin_currency,omitempty"`
	MarketValue    string `json:"market_value,omitempty"`
	StockResponse
	HoldingID   string `json:"holding_id"`
	HoldingType string `json:"holding_type"`
	Side        string `json:"side"`
	Quantity    string `json:"quantity"`
	MarginMode  string `json:"margin_mode,omitempty"`
}

type HoldingsSnapshot struct {
	AccountID   string           `json:"account_id"`
	Holdings    []AccountHolding `json:"holdings"`
	AsOf        string           `json:"as_of"`
	Stale       bool             `json:"stale"`
	LastSuccess string           `json:"last_success,omitempty"`
	Complete    bool             `json:"complete"`
	Warnings    []string         `json:"warnings"`
}

type AccountHandler struct {
	cfg      config.BitgetConfig
	private  accountReader
	market   *bitget.Client
	mu       sync.Mutex
	snapshot *HoldingsSnapshot
	cachedAt time.Time
	flight   chan struct{}
	latest   *HoldingsSnapshot
	lastErr  error
	retryAt  time.Time
}

func NewAccountHandler(cfg config.BitgetConfig) *AccountHandler {
	return &AccountHandler{cfg: cfg, private: bitget.NewAccountClient(bitget.Credentials{APIKey: cfg.APIKey, Secret: cfg.APISecret, Passphrase: cfg.Passphrase}), market: bitget.NewClient("")}
}

func (h *AccountHandler) summary(userID uint) AccountSummary {
	// Never expose the server owner's label/account type to another user.
	item := AccountSummary{ID: "bitget", Provider: "bitget", Name: "Bitget", AccountType: "", Status: "not_configured", Message: "账户尚未连接"}
	if !h.cfg.HasCredentials() || h.cfg.OwnerUserID == 0 {
		return item
	}
	if userID != h.cfg.OwnerUserID {
		item.Status = "unbound"
		item.Message = "当前慧投用户未绑定 Bitget 账户"
		return item
	}
	item.Name = h.cfg.AccountName
	item.AccountType = h.cfg.AccountType
	item.Status = "ready"
	item.Message = "已配置，只读同步"
	return item
}

func (h *AccountHandler) GetAccounts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, []AccountSummary{h.summary(c.GetUint("userID"))})
}

func (h *AccountHandler) GetHoldings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Param("accountID") != "bitget" {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	state := h.summary(c.GetUint("userID"))
	if state.Status == "unbound" {
		c.JSON(http.StatusForbidden, gin.H{"error": state.Message})
		return
	}
	if state.Status != "ready" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bitget 账户尚未连接"})
		return
	}

	data, err := h.holdings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *AccountHandler) holdings(ctx context.Context) (HoldingsSnapshot, error) {
	for {
		h.mu.Lock()
		if h.snapshot != nil && time.Since(h.cachedAt) < 10*time.Second {
			data := *h.snapshot
			h.mu.Unlock()
			return data, nil
		}
		if time.Now().Before(h.retryAt) {
			data, err := h.latest, h.lastErr
			h.mu.Unlock()
			if data != nil {
				return *data, nil
			}
			return HoldingsSnapshot{}, err
		}
		if h.flight == nil {
			h.flight = make(chan struct{})
			go h.syncHoldings()
		}
		done := h.flight
		h.mu.Unlock()
		select {
		case <-ctx.Done():
			return HoldingsSnapshot{}, ctx.Err()
		case <-done:
		}
	}
}

func (h *AccountHandler) syncHoldings() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	data, err := h.readHoldings(ctx)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastErr = err
	if err == nil && data.Complete {
		data.LastSuccess = data.AsOf
		h.snapshot, h.latest = &data, &data
		h.cachedAt = time.Now()
		h.retryAt = time.Time{}
	} else {
		h.retryAt = time.Now().Add(15 * time.Second)
		if h.snapshot != nil {
			stale := *h.snapshot
			stale.Stale, stale.Complete = true, false
			stale.Warnings = append([]string{}, data.Warnings...)
			stale.Warnings = append(stale.Warnings, "本次同步未完成，当前展示上次完整快照")
			if err != nil {
				stale.Warnings = append(stale.Warnings, err.Error())
			}
			h.latest = &stale
		} else if err == nil {
			h.latest = &data
		} else {
			h.latest = nil
		}
	}
	close(h.flight)
	h.flight = nil
}

func (h *AccountHandler) readHoldings(ctx context.Context) (HoldingsSnapshot, error) {
	result := HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{}, Warnings: []string{}, Complete: true}
	var assets []bitget.Asset
	var positions []bitget.Position
	var assetErr, positionErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); assets, assetErr = h.private.Assets(ctx, h.cfg.AccountType) }()
	if h.cfg.IncludeFutures {
		wg.Add(1)
		go func() { defer wg.Done(); positions, positionErr = h.private.Positions(ctx, h.cfg.AccountType) }()
	}
	wg.Wait()
	if assetErr != nil && (!h.cfg.IncludeFutures || positionErr != nil) {
		details := "资产查询：" + bitget.AccountErrorDescription(assetErr)
		if positionErr != nil {
			details += "；合约查询：" + bitget.AccountErrorDescription(positionErr)
		}
		return result, fmt.Errorf("Bitget 持仓读取失败（%s）：%s", h.cfg.AccountType, details)
	}
	if assetErr != nil {
		result.Warnings = append(result.Warnings, "资产查询失败："+bitget.AccountErrorDescription(assetErr)+"；当前列表不完整")
	}
	if positionErr != nil {
		result.Warnings = append(result.Warnings, "合约查询失败："+bitget.AccountErrorDescription(positionErr)+"；当前列表不完整")
	}
	// Fetch inventory only for nonzero holdings. A genuine empty account can
	// return an empty list even if the public market API is temporarily offline.
	for _, productCategory := range []string{"SPOT", "USDT-FUTURES"} {
		if productCategory == "SPOT" {
			nonzero := false
			for _, asset := range assets {
				if bitget.PositiveQuantity(bitget.AssetQuantity(asset, h.cfg.AccountType)) {
					nonzero = true
					break
				}
			}
			if !nonzero {
				continue
			}
		}
		if productCategory == "USDT-FUTURES" {
			nonzero := false
			for _, p := range positions {
				if bitget.PositiveQuantity(p.Total) {
					nonzero = true
					break
				}
			}
			if !nonzero {
				continue
			}
		}
		inventory, err := h.market.StockInventory(ctx, productCategory)
		if err != nil {
			result.Warnings = append(result.Warnings, productCategory+" 股票资料暂不可用，无法完整识别股票持仓")
			continue
		}
		byCoin := make(map[string]bitget.Instrument)
		bySymbol := make(map[string]bitget.Instrument)
		for _, item := range inventory {
			bySymbol[item.Symbol] = item
			coin := strings.ToUpper(item.BaseCoin)
			if _, ok := byCoin[coin]; !ok || item.QuoteCoin == "USDT" {
				byCoin[coin] = item
			}
		}
		var pending []AccountHolding
		if productCategory == "SPOT" {
			for _, asset := range assets {
				quantity := bitget.AssetQuantity(asset, h.cfg.AccountType)
				if !bitget.PositiveQuantity(quantity) {
					continue
				}
				item, ok := byCoin[strings.ToUpper(asset.Coin)]
				if !ok {
					continue
				}
				stock := bitgetStock(bitget.HoldingIdentity(item, productCategory))
				unavailable := false
				stock.QuoteAvailable = &unavailable
				pending = append(pending, AccountHolding{StockResponse: stock, HoldingID: "spot:" + item.Symbol, HoldingType: "spot", Side: "long", Quantity: quantity})
			}
		} else {
			for _, position := range positions {
				if !bitget.PositiveQuantity(position.Total) {
					continue
				}
				item, ok := bySymbol[position.Symbol]
				if !ok {
					continue
				}
				stock := bitgetStock(bitget.HoldingIdentity(item, productCategory))
				unavailable := false
				stock.QuoteAvailable = &unavailable
				pending = append(pending, AccountHolding{StockResponse: stock, HoldingID: "futures:" + position.Symbol + ":" + position.Side + ":" + position.MarginMode, HoldingType: "futures", Side: position.Side, Quantity: position.Total, MarginMode: position.MarginMode, EntryPrice: position.EntryPrice, UnrealizedPnL: position.UnrealizedPnL, MarginCurrency: position.MarginCoin})
			}
		}
		if len(pending) > 0 {
			tickers, err := h.market.InventoryTickers(ctx, productCategory)
			if err != nil {
				result.Warnings = append(result.Warnings, "部分持仓的最新行情暂不可用，持仓数量已保留")
			}
			for i := range pending {
				if ticker, ok := tickers[pending[i].ProviderSymbol]; ok {
					quote := bitget.ApplyTicker(bitget.Quote{Code: pending[i].ID, Name: pending[i].Name, Market: pending[i].Market, ProviderSymbol: pending[i].ProviderSymbol, Currency: pending[i].Currency, ProductType: pending[i].ProductType}, ticker)
					if quote.Price > 0 {
						pending[i].StockResponse = bitgetStock(quote)
						if pending[i].HoldingType == "spot" {
							pending[i].MarketValue = bitget.QuantityValue(pending[i].Quantity, quote.Price)
						}
					}
				}
			}
		}
		result.Holdings = append(result.Holdings, pending...)
	}
	sort.Slice(result.Holdings, func(i, j int) bool {
		a, b := result.Holdings[i], result.Holdings[j]
		if a.Market != b.Market {
			return a.Market < b.Market
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.HoldingID < b.HoldingID
	})
	result.Complete = len(result.Warnings) == 0
	result.AsOf = time.Now().UTC().Format(time.RFC3339)
	return result, nil
}
