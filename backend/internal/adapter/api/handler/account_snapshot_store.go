package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const bitgetSnapshotProvider = "bitget"

type holdingsSnapshotStore interface {
	Load(context.Context, string, uint) (HoldingsSnapshot, bool, error)
	Save(context.Context, string, uint, HoldingsSnapshot) error
}

type gormHoldingsSnapshotStore struct{ db *gorm.DB }

func (s gormHoldingsSnapshotStore) Load(ctx context.Context, provider string, owner uint) (HoldingsSnapshot, bool, error) {
	var row model.AccountHoldingsSnapshot
	err := s.db.WithContext(ctx).Where("provider = ? AND owner_user_id = ?", provider, owner).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return HoldingsSnapshot{}, false, nil
	}
	if err != nil {
		return HoldingsSnapshot{}, false, err
	}
	var snapshot HoldingsSnapshot
	if err := json.Unmarshal([]byte(row.Payload), &snapshot); err != nil {
		return HoldingsSnapshot{}, false, fmt.Errorf("decode account snapshot: %w", err)
	}
	if err := validateCompleteSnapshot(snapshot); err != nil {
		return HoldingsSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s gormHoldingsSnapshotStore) Save(ctx context.Context, provider string, owner uint, snapshot HoldingsSnapshot) error {
	if err := validateCompleteSnapshot(snapshot); err != nil {
		return err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode account snapshot: %w", err)
	}
	capturedAt, err := time.Parse(time.RFC3339, snapshot.AsOf)
	if err != nil {
		return fmt.Errorf("parse account snapshot time: %w", err)
	}
	row := model.AccountHoldingsSnapshot{Provider: provider, OwnerUserID: owner, Payload: string(payload), CapturedAt: capturedAt}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "owner_user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "captured_at", "updated_at"}),
	}).Create(&row).Error
}

func validateCompleteSnapshot(snapshot HoldingsSnapshot) error {
	if !snapshot.Complete || snapshot.Stale || snapshot.AsOf == "" || snapshot.Holdings == nil {
		return fmt.Errorf("account snapshot must be a complete, fresh response")
	}
	if _, err := time.Parse(time.RFC3339, snapshot.AsOf); err != nil {
		return fmt.Errorf("invalid account snapshot time: %w", err)
	}
	return nil
}

// ConfigureSnapshotPersistence restores the last complete Bitget response.
// When the database is empty, an inline environment JSON value is preferred;
// the private file option remains available for backwards compatibility.
func (h *AccountHandler) ConfigureSnapshotPersistence(ctx context.Context, db *gorm.DB) error {
	if h.cfg.OwnerUserID == 0 {
		return nil
	}
	return h.configureSnapshotStore(ctx, gormHoldingsSnapshotStore{db: db}, h.cfg.HoldingsSnapshotJSON, h.cfg.HoldingsSnapshotFile)
}

func (h *AccountHandler) configureSnapshotStore(ctx context.Context, store holdingsSnapshotStore, seedJSON, seedFile string) error {
	h.snapshotStore = store
	snapshot, found, err := store.Load(ctx, bitgetSnapshotProvider, h.cfg.OwnerUserID)
	if err != nil {
		return fmt.Errorf("load Bitget holdings snapshot: %w", err)
	}
	if !found && (seedJSON != "" || seedFile != "") {
		raw := []byte(seedJSON)
		if seedJSON == "" {
			var readErr error
			raw, readErr = os.ReadFile(seedFile)
			if readErr != nil {
				return fmt.Errorf("read Bitget holdings seed: %w", readErr)
			}
		}
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return fmt.Errorf("decode Bitget holdings seed: %w", err)
		}
		if err := validateCompleteSnapshot(snapshot); err != nil {
			return fmt.Errorf("invalid Bitget holdings seed: %w", err)
		}
		if err := store.Save(ctx, bitgetSnapshotProvider, h.cfg.OwnerUserID, snapshot); err != nil {
			return fmt.Errorf("import Bitget holdings seed: %w", err)
		}
		found = true
	}
	if found {
		restored := cloneHoldingsSnapshot(snapshot)
		h.snapshot = &restored
		h.latest = &restored
		// cachedAt intentionally remains zero: startup should attempt a live
		// refresh before falling back to this durable snapshot.
	}
	return nil
}

func cloneHoldingsSnapshot(snapshot HoldingsSnapshot) HoldingsSnapshot {
	copy := snapshot
	copy.Equities = append([]AccountEquity(nil), snapshot.Equities...)
	copy.Accounts = append([]AccountSyncState(nil), snapshot.Accounts...)
	copy.Holdings = append([]AccountHolding(nil), snapshot.Holdings...)
	copy.Warnings = append([]string(nil), snapshot.Warnings...)
	return copy
}
