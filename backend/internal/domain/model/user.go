package model

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Username     string `gorm:"uniqueIndex;not null" json:"username"`
	Email        string `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string `gorm:"not null" json:"-"`
	
	// Profile
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	
	// Settings
	Preferences JSONB `gorm:"type:jsonb" json:"preferences"`
	
	// Relations
	Conversations []Conversation `gorm:"foreignKey:UserID" json:"-"`
}

// TableName specifies the table name for User
func (User) TableName() string {
	return "users"
}
