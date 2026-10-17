package model

import "time"

// Each target is scoped to a user and an exact watchlist symbol or account holding.
type PriceAlert struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UserID        uint       `gorm:"uniqueIndex:idx_price_alert_identity;not null;index" json:"-"`
	HoldingID     string     `gorm:"size:200;uniqueIndex:idx_price_alert_identity;not null" json:"holding_id"`
	AccountID     string     `gorm:"size:32;not null" json:"account_id"`
	Name          string     `gorm:"size:200" json:"name"`
	Code          string     `gorm:"size:100" json:"code"`
	Currency      string     `gorm:"size:20" json:"currency"`
	TargetPrice   float64    `gorm:"not null" json:"target_price"`
	Direction     string     `gorm:"size:12;not null" json:"direction"`
	TriggerMode   string     `gorm:"size:12;not null;default:auto" json:"trigger_mode"`
	Status        string     `gorm:"size:20;not null;index" json:"status"`
	Revision      uint       `gorm:"not null;default:1" json:"revision"`
	LastPrice     *float64   `json:"last_price"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	TriggeredAt   *time.Time `json:"triggered_at"`
	SentAt        *time.Time `json:"sent_at"`
	LastError     string     `gorm:"size:250" json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
type PriceAlertSettings struct {
	UserID    uint      `gorm:"primaryKey" json:"-"`
	Email     string    `gorm:"size:254;not null" json:"email"`
	UpdatedAt time.Time `json:"updated_at"`
}
