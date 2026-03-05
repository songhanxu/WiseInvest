package repository

import (
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeviceTokenRepository handles persistence for device push tokens.
type DeviceTokenRepository struct {
	db *gorm.DB
}

// NewDeviceTokenRepository creates a new DeviceTokenRepository.
func NewDeviceTokenRepository(db *gorm.DB) *DeviceTokenRepository {
	return &DeviceTokenRepository{db: db}
}

// Save upserts a device token. If the token already exists it updates the
// user_id and platform fields, preventing duplicate rows for the same device.
func (r *DeviceTokenRepository) Save(dt *model.DeviceToken) error {
	result := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "platform", "updated_at"}),
	}).Create(dt)
	if result.Error != nil {
		return fmt.Errorf("device_token: save failed: %w", result.Error)
	}
	return nil
}

// FindAll returns all device tokens stored in the database.
func (r *DeviceTokenRepository) FindAll() ([]model.DeviceToken, error) {
	var tokens []model.DeviceToken
	if err := r.db.Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("device_token: find all failed: %w", err)
	}
	return tokens, nil
}

// DeleteByToken removes a specific device token (e.g. on logout or APNs feedback).
func (r *DeviceTokenRepository) DeleteByToken(token string) error {
	if err := r.db.Where("token = ?", token).Delete(&model.DeviceToken{}).Error; err != nil {
		return fmt.Errorf("device_token: delete failed: %w", err)
	}
	return nil
}
