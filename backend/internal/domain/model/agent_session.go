package model

import (
	"time"

	"gorm.io/gorm"
)

// AgentSession represents an agent execution session
type AgentSession struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	ConversationID uint   `gorm:"not null;index" json:"conversation_id"`
	AgentType      string `gorm:"not null" json:"agent_type"`
	
	// Session state
	State  map[string]interface{} `gorm:"type:jsonb" json:"state"`
	Status string                 `gorm:"default:'running'" json:"status"` // running, completed, failed
	
	// Execution metadata
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	
	// Relations
	Conversation Conversation `gorm:"foreignKey:ConversationID" json:"-"`
}

// TableName specifies the table name for AgentSession
func (AgentSession) TableName() string {
	return "agent_sessions"
}

// SessionStatus constants
const (
	SessionStatusRunning   = "running"
	SessionStatusCompleted = "completed"
	SessionStatusFailed    = "failed"
)
