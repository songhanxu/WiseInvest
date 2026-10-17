package database

import (
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPostgresDB creates a new PostgreSQL database connection
func NewPostgresDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := cfg.DSN()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL database
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	return db, nil
}

// AutoMigrate runs database migrations
func AutoMigrate(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&model.WatchlistItem{}) && !tx.Migrator().HasIndex(&model.WatchlistItem{}, "idx_watchlist_identity") {
			if err := tx.Exec(`LOCK TABLE watchlist_items IN ACCESS EXCLUSIVE MODE`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE watchlist_items SET stock_code = CASE
 WHEN market='crypto' AND LOWER(TRIM(stock_code))='btc_price' THEN 'BTC'
 WHEN market='crypto' AND LOWER(TRIM(stock_code))='eth_price' THEN 'ETH'
 WHEN market='crypto' THEN REGEXP_REPLACE(UPPER(TRIM(stock_code)), '/?USDT$', '')
 WHEN market='us_stock' THEN UPPER(TRIM(stock_code))
 WHEN market='hk_stock' AND REGEXP_REPLACE(UPPER(TRIM(stock_code)), '^HK|[.]HK$', '', 'g') ~ '^[0-9]{1,5}$' THEN LPAD(REGEXP_REPLACE(UPPER(TRIM(stock_code)), '^HK|[.]HK$', '', 'g'),5,'0')
 WHEN market='a_share' AND TRIM(stock_code) ~ '^[0-9]{6}$' THEN (CASE WHEN LOWER(symbol) LIKE 'sz%' OR (symbol='' AND LEFT(stock_code,1) IN ('0','3')) THEN 'sz' ELSE 'sh' END)||TRIM(stock_code)
 WHEN market='a_share' THEN LOWER(TRIM(stock_code))
 ELSE TRIM(stock_code) END`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DELETE FROM watchlist_items a USING watchlist_items b WHERE a.user_id=b.user_id AND a.market=b.market AND a.stock_code=b.stock_code AND a.id>b.id`).Error; err != nil {
				return err
			}
		}
		if err := tx.AutoMigrate(
			&model.User{},
			&model.Conversation{},
			&model.Message{},
			&model.AgentSession{},
			&model.DeviceToken{},
			&model.WatchlistItem{},
			&model.PriceAlert{},
			&model.PriceAlertSettings{},
		); err != nil {
			return err
		}
		return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_watchlist_identity ON watchlist_items(user_id,market,stock_code)`).Error
	})
}

// SeedDefaultData creates default data if not exists
func SeedDefaultData(db *gorm.DB) error {
	// Check if demo user already exists by username
	var count int64
	if err := db.Model(&model.User{}).Where("username = ?", "demo_user").Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check default user: %w", err)
	}

	if count == 0 {
		defaultUser := &model.User{
			Username:     "demo_user",
			Email:        "demo@wiseinvest.com",
			PasswordHash: "$2a$10$dummy.hash.for.demo.user.only",
			DisplayName:  "Demo User",
			Preferences:  model.JSONB{},
		}
		if err := db.Create(defaultUser).Error; err != nil {
			return fmt.Errorf("failed to create default user: %w", err)
		}
	}

	// Ensure the users_id_seq is at least at the current max ID so new inserts won't collide
	if err := db.Exec("SELECT setval('users_id_seq', GREATEST((SELECT MAX(id) FROM users), 1))").Error; err != nil {
		// Non-fatal: sequence may have a different name on older installs
		fmt.Printf("Warning: could not reset users_id_seq: %v\n", err)
	}

	return nil
}
