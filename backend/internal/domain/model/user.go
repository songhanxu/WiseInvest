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
	PasswordHash string `json:"-"`

	// Profile
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`

	// WeChat OAuth
	WeChatOpenID   *string `gorm:"uniqueIndex;column:wechat_open_id" json:"wechat_open_id,omitempty"`
	WeChatUnionID  string  `gorm:"column:wechat_union_id" json:"wechat_union_id,omitempty"`
	WeChatNickname string  `gorm:"column:wechat_nickname" json:"wechat_nickname,omitempty"`
	WeChatAvatar   string  `gorm:"column:wechat_avatar" json:"wechat_avatar,omitempty"`

	// Phone (nullable, unique when set)
	Phone *string `gorm:"uniqueIndex" json:"phone,omitempty"`

	// Settings
	Preferences JSONB `gorm:"type:jsonb" json:"preferences"`

	// Relations
	Conversations []Conversation `gorm:"foreignKey:UserID" json:"-"`
}

// TableName specifies the table name for User
func (User) TableName() string {
	return "users"
}
