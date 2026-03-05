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
		&model.DeviceToken{},
	)
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
