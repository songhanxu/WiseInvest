package repository

import (
	"context"
	"errors"
	"time"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type PriceAlertRepository struct{ db *gorm.DB }

func NewPriceAlertRepository(db *gorm.DB) *PriceAlertRepository {
	return &PriceAlertRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}
func (r *PriceAlertRepository) List(ctx context.Context, user uint) ([]model.PriceAlert, error) {
	rows := []model.PriceAlert{}
	err := r.db.WithContext(ctx).Where("user_id = ?", user).Order("id").Find(&rows).Error
	return rows, err
}
func (r *PriceAlertRepository) Settings(ctx context.Context, user uint) (model.PriceAlertSettings, error) {
	var row model.PriceAlertSettings
	err := r.db.WithContext(ctx).Where("user_id = ?", user).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.PriceAlertSettings{UserID: user}, nil
	}
	return row, err
}
func (r *PriceAlertRepository) SaveSettings(ctx context.Context, row *model.PriceAlertSettings) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"email", "updated_at"})}).Create(row).Error
}
func (r *PriceAlertRepository) Save(ctx context.Context, row *model.PriceAlert) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?, ?)", 74001, int(row.UserID)).Error; err != nil {
			return err
		}
		if row.AssetKey != "" {
			var existing model.PriceAlert
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND asset_key = ?", row.UserID, row.AssetKey).Order("updated_at DESC, id DESC").First(&existing).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				if existing.Status == "sending" {
					return errors.New("提醒正在发送，请稍后再修改")
				}
				// Keep one row/revision per asset, even when edited in another
				// account. Source and currency are changed only on explicit save.
				row.ID, row.CreatedAt, row.Revision = existing.ID, existing.CreatedAt, existing.Revision+1
				row.LastCheckedAt, row.TriggeredAt, row.SentAt, row.LastError = nil, nil, nil, ""
				return tx.Save(row).Error
			}
		}
		var count int64
		if err := tx.Model(&model.PriceAlert{}).Where("user_id = ? AND holding_id <> ?", row.UserID, row.HoldingID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 100 {
			return errors.New("最多保存 100 项到价提醒")
		}
		err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "holding_id"}}, DoUpdates: clause.Assignments(map[string]any{"asset_key": row.AssetKey, "account_id": row.AccountID, "name": row.Name, "code": row.Code, "currency": row.Currency, "target_price": row.TargetPrice, "direction": row.Direction, "trigger_mode": row.TriggerMode, "status": "armed", "revision": gorm.Expr("price_alerts.revision + 1"), "last_price": row.LastPrice, "last_checked_at": nil, "triggered_at": nil, "sent_at": nil, "last_error": "", "updated_at": time.Now()})}).Create(row).Error
		if err != nil {
			return err
		}
		return tx.Where("user_id = ? AND holding_id = ?", row.UserID, row.HoldingID).First(row).Error
	})
}

func (r *PriceAlertRepository) ReconcileIdentities(ctx context.Context, user uint, identities map[string]string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?, ?)", 74001, int(user)).Error; err != nil {
			return err
		}
		for holding, asset := range identities {
			if err := tx.Model(&model.PriceAlert{}).Where("user_id = ? AND holding_id = ? AND (asset_key IS NULL OR asset_key = '')", user, holding).UpdateColumn("asset_key", asset).Error; err != nil {
				return err
			}
		}
		var rows []model.PriceAlert
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND asset_key <> ''", user).Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
			return err
		}
		// A currently sending revision cannot be withdrawn. Retain it while
		// retiring duplicates, so the monitor cannot dispatch them as well.
		keepers := map[string]model.PriceAlert{}
		for _, row := range rows {
			previous, exists := keepers[row.AssetKey]
			if !exists || row.Status == "sending" && previous.Status != "sending" {
				keepers[row.AssetKey] = row
			}
		}
		for _, row := range rows {
			if row.ID != keepers[row.AssetKey].ID && row.Status != "sending" {
				if err := tx.Where("id = ? AND user_id = ? AND status <> ?", row.ID, user, "sending").Delete(&model.PriceAlert{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func (r *PriceAlertRepository) Remove(ctx context.Context, user uint, holding string) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND holding_id = ?", user, holding).Delete(&model.PriceAlert{}).Error
}
func (r *PriceAlertRepository) Armed(ctx context.Context) ([]model.PriceAlert, error) {
	rows := []model.PriceAlert{}
	err := r.db.WithContext(ctx).Where("status = ? AND user_id IN (?)", "armed", r.db.Model(&model.PriceAlertSettings{}).Select("user_id").Where("email <> ?", "")).Order("last_checked_at ASC NULLS FIRST, id ASC").Limit(500).Find(&rows).Error
	return rows, err
}
func (r *PriceAlertRepository) Checked(ctx context.Context, row model.PriceAlert, price *float64, message string) error {
	return r.db.WithContext(ctx).Model(&model.PriceAlert{}).Where("id = ? AND revision = ? AND status = ?", row.ID, row.Revision, "armed").Updates(map[string]any{"last_price": price, "last_checked_at": time.Now(), "last_error": message}).Error
}
func (r *PriceAlertRepository) Claim(ctx context.Context, row model.PriceAlert, price float64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.PriceAlert{}).Where("id = ? AND revision = ? AND status = ?", row.ID, row.Revision, "armed").Updates(map[string]any{"status": "sending", "triggered_at": time.Now(), "last_price": price, "last_checked_at": time.Now(), "last_error": ""})
	return q.RowsAffected == 1, q.Error
}
func (r *PriceAlertRepository) Delivered(ctx context.Context, row model.PriceAlert, sendErr error) error {
	values := map[string]any{"status": "sent", "sent_at": time.Now(), "last_error": ""}
	if sendErr != nil {
		values = map[string]any{"status": "error", "last_error": "邮件发送失败，请检查发信配置后重新保存目标价启用"}
	}
	return r.db.WithContext(ctx).Model(&model.PriceAlert{}).Where("id = ? AND revision = ? AND status = ?", row.ID, row.Revision, "sending").Updates(values).Error
}
func (r *PriceAlertRepository) RecoverInterrupted(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.PriceAlert{}).Where("status = ? AND updated_at < ?", "sending", time.Now().Add(-10*time.Minute)).Updates(map[string]any{"status": "error", "last_error": "发送结果待确认，请检查收件箱后重新保存目标价启用"}).Error
}
