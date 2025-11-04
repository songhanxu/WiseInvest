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
	return db.AutoMigrate(
		&model.User{},
		&model.Conversation{},
		&model.Message{},
		&model.AgentSession{},
	)
}

// SeedDefaultData creates default data if not exists
func SeedDefaultData(db *gorm.DB) error {
	// Check if default user exists
	var count int64
	if err := db.Model(&model.User{}).Where("id = ?", 1).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check default user: %w", err)
	}

	// Create default user if not exists
	if count == 0 {
		defaultUser := &model.User{
			ID:           1,
			Username:     "demo_user",
			Email:        "demo@wiseinvest.com",
			PasswordHash: "$2a$10$dummy.hash.for.demo.user.only", // Dummy hash for demo
			DisplayName:  "Demo User",
			Avatar:       "",
			Preferences:  model.JSONB{},
		}

		if err := db.Create(defaultUser).Error; err != nil {
			return fmt.Errorf("failed to create default user: %w", err)
		}
	}

	return nil
}
