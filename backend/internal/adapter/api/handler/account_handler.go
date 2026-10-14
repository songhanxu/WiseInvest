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
	StockResponse
	HoldingID   string `json:"holding_id"`
	HoldingType string `json:"holding_type"`
	Side        string `json:"side"`
	Quantity    string `json:"quantity"`
	MarginMode  string `json:"margin_mode,omitempty"`
}

type HoldingsSnapshot struct {
	AccountID string           `json:"account_id"`
	Holdings  []AccountHolding `json:"holdings"`
	AsOf      string           `json:"as_of"`
	Complete  bool             `json:"complete"`
	Warnings  []string         `json:"warnings"`
}

type AccountHandler struct {
	cfg      config.BitgetConfig
	private  accountReader
	market   *bitget.Client
	mu       sync.Mutex
	snapshot *HoldingsSnapshot
	cachedAt time.Time
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	// Cache is reachable only after ownership checks, including on cache hits.
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.snapshot != nil && time.Since(h.cachedAt) < 10*time.Second {
		c.JSON(http.StatusOK, h.snapshot)
		return
	}
	data, err := h.readHoldings(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if data.Complete {
		h.snapshot = &data
		h.cachedAt = time.Now()
	}
	c.JSON(http.StatusOK, data)
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
		return result, fmt.Errorf("Bitget 持仓读取失败，请检查 API 凭证、只读权限、IP 白名单和账户类型")
	}
	if assetErr != nil {
		result.Warnings = append(result.Warnings, "现货持仓暂未读取，当前列表不完整")
	}
	if positionErr != nil {
		result.Warnings = append(result.Warnings, "合约持仓暂未读取，请检查合约查询权限；当前列表不完整")
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
				pending = append(pending, AccountHolding{StockResponse: stock, HoldingID: "futures:" + position.Symbol + ":" + position.Side + ":" + position.MarginMode, HoldingType: "futures", Side: position.Side, Quantity: position.Total, MarginMode: position.MarginMode})
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
