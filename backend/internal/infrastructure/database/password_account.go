package database

import (
	"errors"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	ormlog "gorm.io/gorm/logger"
)

// BootstrapPasswordAccount preserves the selected user's ID and all associations.
// A valid existing password is never reset on subsequent application restarts.
func BootstrapPasswordAccount(db *gorm.DB, cfg config.AccountBootstrapConfig) error {
	if cfg.UserID == 0 {
		return nil
	}
	username, err := auth.NormalizeUsername(cfg.Username)
	if err != nil {
		return err
	}
	return db.Session(&gorm.Session{Logger: ormlog.Default.LogMode(ormlog.Silent)}).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, cfg.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("selected bootstrap user does not exist; no account was created")
			}
			return fmt.Errorf("failed to read bootstrap account")
		}
		if auth.HasPassword(user.PasswordHash) {
			if user.Username != username {
				return fmt.Errorf("selected user already has password credentials; refusing to overwrite them")
			}
			return nil
		}
		var count int64
		if err := tx.Unscoped().Model(&model.User{}).Where("LOWER(username) = ? AND id <> ?", username, user.ID).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to check bootstrap username")
		}
		if count != 0 {
			return fmt.Errorf("bootstrap username is already used by another account; no user data changed")
		}
		hash, err := auth.HashPassword(cfg.Password)
		if err != nil {
			return err
		}
		if err := tx.Model(&user).Updates(map[string]interface{}{"username": username, "password_hash": hash}).Error; err != nil {
			return fmt.Errorf("failed to configure bootstrap credentials")
		}
		return nil
	})
}
