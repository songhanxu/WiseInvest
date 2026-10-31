package model

import "time"

// AccountHoldingsSnapshot stores the last complete account response. It is
// deliberately provider-scoped so a failed upstream request can never replace
// a good snapshot from another exchange or another WiseInvest user.
type AccountHoldingsSnapshot struct {
	ID          uint      `gorm:"primaryKey"`
	Provider    string    `gorm:"size:32;uniqueIndex:idx_account_holdings_snapshot;not null"`
	OwnerUserID uint      `gorm:"uniqueIndex:idx_account_holdings_snapshot;not null"`
	Payload     string    `gorm:"type:jsonb;not null"`
	CapturedAt  time.Time `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
