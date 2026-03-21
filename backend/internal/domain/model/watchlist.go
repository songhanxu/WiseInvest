package model

import "time"

// WatchlistItem represents a user's watchlisted stock, bound to their UserID.
type WatchlistItem struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index:idx_watchlist_user_market;not null"`
	Market    string    `json:"market" gorm:"index:idx_watchlist_user_market;size:20;not null"` // a_share, us_stock, crypto
	StockCode string   `json:"stock_code" gorm:"size:20;not null"`                             // e.g. "600519", "AAPL", "BTC"
	Symbol    string    `json:"symbol" gorm:"size:30;not null"`                                 // e.g. "SH600519", "AAPL", "BTC/USDT"
	Name      string    `json:"name" gorm:"size:100;not null"`
	SortOrder int       `json:"sort_order" gorm:"default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (WatchlistItem) TableName() string { return "watchlist_items" }
