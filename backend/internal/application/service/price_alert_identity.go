package service

import (
	"context"
	"strings"
	"time"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
)

type alertIdentityStore interface {
	ReconcileIdentities(context.Context, uint, map[string]string) error
}

func legacyWatchlistAsset(holding string) string {
	if !strings.HasPrefix(holding, "watchlist:") {
		return ""
	}
	key := strings.TrimPrefix(holding, "watchlist:")
	parts := strings.Split(key, ":")
	if len(parts) != 2 || parts[1] == "" {
		return ""
	}
	switch parts[0] {
	case "a_share", "us_stock", "hk_stock", "crypto":
		return key
	}
	return ""
}

// Resolve only the current user's owned holdings. Linking old rows changes no
// thresholds, currencies, direction, revision or sent status. Unavailable
// sources are left untouched until a subsequent successful lookup.
func (s *PriceAlertService) List(ctx context.Context, user uint) ([]model.PriceAlert, error) {
	rows, err := s.Store.List(ctx, user)
	if err != nil {
		return nil, err
	}
	store, supported := s.Store.(alertIdentityStore)
	if !supported {
		return rows, nil
	}
	identities := map[string]string{}
	accounts := map[string]bool{}
	for _, row := range rows {
		if row.AssetKey != "" {
			continue
		}
		if key := legacyWatchlistAsset(row.HoldingID); key != "" {
			identities[row.HoldingID] = key
		} else {
			accounts[row.AccountID] = true
		}
	}
	for account := range accounts {
		query, cancel := context.WithTimeout(ctx, 5*time.Second)
		quotes, readErr := s.Quotes(query, user, account)
		cancel()
		if readErr != nil {
			continue
		}
		for _, q := range quotes {
			if q.AssetKey != "" {
				identities[q.HoldingID] = q.AssetKey
			}
		}
	}
	if len(identities) == 0 {
		return rows, nil
	}
	if err := store.ReconcileIdentities(ctx, user, identities); err != nil {
		return nil, err
	}
	return s.Store.List(ctx, user)
}
