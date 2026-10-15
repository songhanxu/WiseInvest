package repository

import (
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// WatchlistRepository handles database operations for user watchlists.
type WatchlistRepository struct {
	db *gorm.DB
}

// NewWatchlistRepository creates a new WatchlistRepository.
func NewWatchlistRepository(db *gorm.DB) *WatchlistRepository {
	return &WatchlistRepository{db: db}
}

func (r *WatchlistRepository) GetByUser(userID uint) ([]model.WatchlistItem, error) {
	var items []model.WatchlistItem
	err := r.db.Where("user_id = ?", userID).Order("market ASC, sort_order ASC, id ASC").Find(&items).Error
	return items, err
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
	// Get max sort_order for this user+market
	var maxOrder int
	err := r.db.Model(&model.WatchlistItem{}).
		Where("user_id = ? AND market = ?", item.UserID, item.Market).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error
	if err != nil {
		return err
	}
	item.SortOrder = maxOrder + 1

	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "market"}, {Name: "stock_code"}}, DoNothing: true}).Create(item).Error
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

func (r *WatchlistRepository) RemoveCrypto(userID uint, code string) error {
	aliases := []string{code, code + "USDT", code + "/USDT"}
	legacy := map[string]string{"BTC": "btc_price", "ETH": "eth_price", "SOL": "sol_price", "BNB": "bnb_price"}
	if value := legacy[code]; value != "" {
		aliases = append(aliases, value)
	}
	for i := range aliases {
		aliases[i] = strings.ToUpper(aliases[i])
	}
	return r.db.Where("user_id = ? AND market = ? AND UPPER(stock_code) IN ?", userID, "crypto", aliases).Delete(&model.WatchlistItem{}).Error
}
