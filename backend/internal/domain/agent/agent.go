package agent

import (
	"context"
)

// Agent represents an AI agent interface
type Agent interface {
	// GetType returns the agent type
	GetType() string
	
	// GetSystemPrompt returns the system prompt for the agent
	GetSystemPrompt() string
	
	// Process processes a user message and returns a response
	Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error)
	
	// ProcessStream processes a user message and streams the response
	ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error
}

// ProcessRequest represents a request to process a message
type ProcessRequest struct {
	UserMessage      string
	ConversationID   uint
	ConversationHistory []HistoryMessage
	Context          map[string]interface{}
}

// ProcessResponse represents a response from processing a message
type ProcessResponse struct {
	Content          string
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Metadata         map[string]interface{}
}

// HistoryMessage represents a message in conversation history
type HistoryMessage struct {
	Role    string
	Content string
}

// AgentType constants
const (
	// Legacy agent types (kept for backward compatibility)
	TypeOrchestrator      = "orchestrator"
	TypeConversation      = "conversation"
	TypeInvestmentAdvisor = "investment_advisor" // Deprecated: use TypeAShare
	TypeTradingAgent      = "trading_agent"
	TypeTrading           = "trading"

	// Market-based agent types
	TypeAShare  = "a_share"  // A股：沪深北交所
	TypeUSStock = "us_stock" // 美股：纽交所/纳斯达克
	TypeCrypto  = "crypto"   // 币圈：加密货币
)
