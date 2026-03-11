package model

import "time"

// DeviceToken stores an iOS/Android device push token associated with a user.
type DeviceToken struct {
	ID        uint      `gorm:"primarykey"                 json:"id"`
	CreatedAt time.Time `                                  json:"created_at"`
	UpdatedAt time.Time `                                  json:"updated_at"`

	UserID   uint   `gorm:"index;not null"             json:"user_id"`
	Token    string `gorm:"uniqueIndex;not null;size:200" json:"token"`
	Platform string `gorm:"not null;default:'ios'"     json:"platform"`
}
