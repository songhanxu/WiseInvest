package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/application/service"
)

type PriceAlertHandler struct{ Service *service.PriceAlertService }

func (h *PriceAlertHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user := c.GetUint("userID")
	rows, err := h.Service.List(c.Request.Context(), user)
	if err != nil {
		c.JSON(500, gin.H{"error": "提醒列表读取失败"})
		return
	}
	settings, err := h.Service.Store.Settings(c.Request.Context(), user)
	if err != nil {
		c.JSON(500, gin.H{"error": "提醒邮箱读取失败"})
		return
	}
	c.JSON(200, gin.H{"alerts": rows, "email": settings.Email, "mailer_configured": h.Service.MailerReady(), "check_interval_seconds": 60})
}
func (h *PriceAlertHandler) Settings(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "邮箱格式无效"})
		return
	}
	if err := h.Service.SaveEmail(c.Request.Context(), c.GetUint("userID"), req.Email); err != nil {
		c.JSON(400, gin.H{"error": "邮箱保存失败，请检查邮箱格式"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (h *PriceAlertHandler) Save(c *gin.Context) {
	var req struct {
		AccountID   string  `json:"account_id"`
		HoldingID   string  `json:"holding_id"`
		TargetPrice float64 `json:"target_price"`
		Direction   string  `json:"direction"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "请输入有效的期望价格"})
		return
	}
	if req.Direction == "" {
		req.Direction = "auto"
	}
	row, err := h.Service.Save(c.Request.Context(), c.GetUint("userID"), req.AccountID, req.HoldingID, req.TargetPrice, req.Direction)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, row)
}
func (h *PriceAlertHandler) Remove(c *gin.Context) {
	holding := c.Query("holding_id")
	if holding == "" || len(holding) > 200 {
		c.JSON(400, gin.H{"error": "holding_id 无效"})
		return
	}
	if h.Service.Store.Remove(c.Request.Context(), c.GetUint("userID"), holding) != nil {
		c.JSON(500, gin.H{"error": "提醒删除失败"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// Every monitor lookup repeats the same ownership guard used by the HTTP API.
// Cached or stale data cannot bypass account binding or trigger an alert.
func (h *AccountHandler) PriceAlertQuotes(ctx context.Context, user uint, accountID string) ([]service.AlertQuote, error) {
	target := h
	if accountID != "bitget" {
		target = h.others[accountID]
	}
	if target == nil {
		return nil, errAlertAccount
	}
	state := target.summary(user)
	if state.Status != "ready" {
		return nil, errAlertAccount
	}
	snapshot, err := target.holdings(ctx)
	if err != nil {
		return nil, err
	}
	fresh := !snapshot.Stale
	stamp, err := time.Parse(time.RFC3339, snapshot.AsOf)
	if err != nil || time.Since(stamp) > 2*time.Minute || stamp.After(time.Now().Add(time.Minute)) {
		fresh = false
	}
	rows := []service.AlertQuote{}
	for _, row := range decorateSnapshot(snapshot, state).Holdings {
		available := row.QuoteAvailable == nil || *row.QuoteAvailable
		quoteFresh := fresh
		quoteFresh = quoteFresh && alertQuoteTimeFresh(row.QuoteTime)
		assetKey := alertAssetKey(row.Market, row.ID, row.Symbol)
		if assetKey == "" {
			assetKey = "holding:" + row.HoldingID
		}
		rows = append(rows, service.AlertQuote{HoldingID: row.HoldingID, AssetKey: assetKey, AccountID: state.ID, Name: row.Name, Code: row.ID, Currency: row.Currency, Price: row.CurrentPrice, Fresh: quoteFresh && available})
	}
	return rows, nil
}

var errAlertAccount = errors.New("账户未连接或未绑定当前用户")

func alertQuoteTimeFresh(value string) bool {
	if value == "" {
		return true // A successful quote lookup can omit a provider timestamp.
	}
	var stamp time.Time
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
		stamp = time.UnixMilli(millis)
	} else {
		stamp, _ = time.Parse(time.RFC3339, value)
	}
	return !stamp.IsZero() && time.Since(stamp) <= 5*time.Minute && !stamp.After(time.Now().Add(time.Minute))
}

// Read only this user's saved symbols. Watchlist monitoring uses public quotes,
// independently of exchange account credentials or private holdings.
func (h *StockHandler) WatchlistPriceAlertQuotes(ctx context.Context, user uint) ([]service.AlertQuote, error) {
	if user == 0 {
		return nil, errors.New("请先登录")
	}
	stocks, err := h.readWatchlist(ctx, user)
	if err != nil {
		return nil, err
	}
	quotes := make([]service.AlertQuote, 0, len(stocks))
	for _, stock := range stocks {
		currency := stock.Currency
		if currency == "" {
			currency = map[string]string{"a_share": "CNY", "us_stock": "USD", "hk_stock": "HKD", "crypto": "USDT"}[stock.Market]
		}
		quotes = append(quotes, service.AlertQuote{
			HoldingID: fmt.Sprintf("watchlist:%s:%s", stock.Market, canonicalWatchCode(stock.Market, stock.ID, stock.Symbol)),
			AssetKey:  alertAssetKey(stock.Market, stock.ID, stock.Symbol),
			AccountID: "watchlist", Name: stock.Name, Code: stock.ID, Currency: currency, Price: stock.CurrentPrice,
			Fresh: (stock.QuoteAvailable == nil || *stock.QuoteAvailable) && alertQuoteTimeFresh(stock.QuoteTime),
		})
	}
	return quotes, nil
}

func alertAssetKey(market, code, symbol string) string {
	switch market {
	case "a_share", "us_stock", "hk_stock", "crypto":
		return market + ":" + canonicalWatchCode(market, code, symbol)
	}
	// Unknown products must not accidentally share a stock's target.
	return ""
}
