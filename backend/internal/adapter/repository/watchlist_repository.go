package repository

import (
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
)

// WatchlistRepository handles database operations for user watchlists.
type WatchlistRepository struct {
	db *gorm.DB
}

// NewWatchlistRepository creates a new WatchlistRepository.
func NewWatchlistRepository(db *gorm.DB) *WatchlistRepository {
	return &WatchlistRepository{db: db}
}

// GetByUserAndMarket returns all watchlist items for a user in a specific market, ordered by sort_order.
func (r *WatchlistRepository) GetByUserAndMarket(userID uint, market string) ([]model.WatchlistItem, error) {
	var items []model.WatchlistItem
	err := r.db.Where("user_id = ? AND market = ?", userID, market).
		Order("sort_order ASC, id ASC").
		Find(&items).Error
	return items, err
}

// Add adds a stock to the user's watchlist. Returns the created item.
func (r *WatchlistRepository) Add(item *model.WatchlistItem) error {
	// Check if already exists
	var count int64
	r.db.Model(&model.WatchlistItem{}).
		Where("user_id = ? AND market = ? AND stock_code = ?", item.UserID, item.Market, item.StockCode).
		Count(&count)
	if count > 0 {
		return nil // already in watchlist
	}

	// Get max sort_order for this user+market
	var maxOrder int
	r.db.Model(&model.WatchlistItem{}).
		Where("user_id = ? AND market = ?", item.UserID, item.Market).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder)
	item.SortOrder = maxOrder + 1

	return r.db.Create(item).Error
}

// Remove removes a stock from the user's watchlist.
// Returns nil even if no rows were affected (idempotent).
func (r *WatchlistRepository) Remove(userID uint, market string, stockCode string) error {
	return r.db.Where("user_id = ? AND market = ? AND stock_code = ?", userID, market, stockCode).
		Delete(&model.WatchlistItem{}).Error
}

// Exists checks if a stock is in the user's watchlist.
func (r *WatchlistRepository) Exists(userID uint, market string, stockCode string) (bool, error) {
	var count int64
	err := r.db.Model(&model.WatchlistItem{}).
		Where("user_id = ? AND market = ? AND stock_code = ?", userID, market, stockCode).
		Count(&count).Error
	return count > 0, err
}
