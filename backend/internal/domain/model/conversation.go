package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// JSONB is a custom type for handling JSONB columns
type JSONB map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = make(map[string]interface{})
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to unmarshal JSONB value")
	}

	result := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}

	*j = result
	return nil
}

// Conversation represents a conversation between user and agent
type Conversation struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	UserID uint   `gorm:"not null;index" json:"user_id"`
	Title  string `json:"title"`
	
	// Agent information
	AgentType string `gorm:"not null" json:"agent_type"` // investment_advisor, trading_agent
	
	// Conversation metadata
	Metadata JSONB `gorm:"type:jsonb" json:"metadata"`
	
	// Status
	Status string `gorm:"default:'active'" json:"status"` // active, archived, deleted
	
	// Relations
	User     User      `gorm:"foreignKey:UserID" json:"-"`
	Messages []Message `gorm:"foreignKey:ConversationID" json:"messages,omitempty"`
}

// TableName specifies the table name for Conversation
func (Conversation) TableName() string {
	return "conversations"
}

// ConversationStatus constants
const (
	ConversationStatusActive   = "active"
	ConversationStatusArchived = "archived"
	ConversationStatusDeleted  = "deleted"
)

// AgentType constants
const (
	AgentTypeInvestmentAdvisor = "investment_advisor"
	AgentTypeTradingAgent      = "trading_agent"
)
