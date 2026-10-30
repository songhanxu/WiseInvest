package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type secSnapshotStore interface {
	Load(context.Context, string) (FundamentalsResponse, time.Time, bool, error)
	Save(context.Context, string, FundamentalsResponse, time.Time) error
	WatchedCodes(context.Context) ([]string, error)
}

type postgresSECSnapshotStore struct{ db *gorm.DB }

func (s postgresSECSnapshotStore) Load(ctx context.Context, code string) (FundamentalsResponse, time.Time, bool, error) {
	var saved model.SECSnapshot
	err := s.db.WithContext(ctx).Where("code = ?", code).First(&saved).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return FundamentalsResponse{}, time.Time{}, false, nil
	}
	if err != nil {
		return FundamentalsResponse{}, time.Time{}, false, err
	}
	var row FundamentalsResponse
	if err := json.Unmarshal([]byte(saved.Payload), &row); err != nil {
		return FundamentalsResponse{}, time.Time{}, false, err
	}
	return row, saved.FetchedAt, true, nil
}

func (s postgresSECSnapshotStore) Save(ctx context.Context, code string, row FundamentalsResponse, at time.Time) error {
	data, err := json.Marshal(row)
	if err != nil {
		return err
	}
	record := model.SECSnapshot{Code: code, Payload: string(data), FetchedAt: at}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "fetched_at", "updated_at"}),
	}).Create(&record).Error
}

func (s postgresSECSnapshotStore) WatchedCodes(ctx context.Context) ([]string, error) {
	var codes []string
	err := s.db.WithContext(ctx).Model(&model.WatchlistItem{}).
		Where("market = ?", "us_stock").Distinct("stock_code").Limit(512).Pluck("stock_code", &codes).Error
	if err != nil {
		return nil, err
	}
	for i := range codes {
		codes[i] = strings.ToUpper(strings.TrimSpace(codes[i]))
	}
	return codes, nil
}
