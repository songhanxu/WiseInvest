package model

import (
	"time"

	"gorm.io/gorm"
)

// Message represents a message in a conversation
type Message struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	ConversationID uint   `gorm:"not null;index" json:"conversation_id"`
	Role           string `gorm:"not null" json:"role"` // user, assistant, system
	Content        string `gorm:"type:text;not null" json:"content"`
	
	// Message metadata
	Metadata JSONB `gorm:"type:jsonb" json:"metadata,omitempty"`
	
	// Token usage (for assistant messages)
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	
	// Relations
	Conversation Conversation `gorm:"foreignKey:ConversationID" json:"-"`
}

// TableName specifies the table name for Message
func (Message) TableName() string {
	return "messages"
}

// MessageRole constants
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleSystem    = "system"
)
