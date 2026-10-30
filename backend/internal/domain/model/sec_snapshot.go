package model

import "time"

// SECSnapshot keeps the last successfully verified SEC response for a symbol.
// Payload includes filing identity, report dates, facts and provenance.
type SECSnapshot struct {
	ID        uint      `gorm:"primaryKey"`
	Code      string    `gorm:"size:30;uniqueIndex;not null"`
	Payload   string    `gorm:"type:jsonb;not null"`
	FetchedAt time.Time `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
