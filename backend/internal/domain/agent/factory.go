package agent

import (
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// Factory creates agents
type Factory struct {
	llmClient *llm.OpenAIClient
	logger    *logger.Logger
}

// NewAgentFactory creates a new agent factory
func NewAgentFactory(llmClient *llm.OpenAIClient, logger *logger.Logger) *Factory {
	return &Factory{
		llmClient: llmClient,
		logger:    logger,
	}
}

// CreateAgent creates an agent by type
func (f *Factory) CreateAgent(agentType string) (Agent, error) {
	switch agentType {
	case TypeInvestmentAdvisor:
		return NewInvestmentAdvisorAgent(f.llmClient, f.logger), nil
	case TypeTradingAgent:
		return NewTradingAgent(f.llmClient, f.logger), nil
	default:
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
}

// GetAvailableAgents returns a list of available agent types
func (f *Factory) GetAvailableAgents() []AgentInfo {
	return []AgentInfo{
		{
			Type:        TypeInvestmentAdvisor,
			Name:        "Investment Advisor",
			Description: "专业的投资分析顾问，提供市场分析、风险评估和投资建议",
			Icon:        "chart.line.uptrend.xyaxis",
			Color:       "#4CAF50",
		},
		{
			Type:        TypeTradingAgent,
			Name:        "Trading Agent",
			Description: "自动化交易助手，连接币安API执行交易操作",
			Icon:        "arrow.left.arrow.right",
			Color:       "#2196F3",
		},
	}
}

// AgentInfo represents agent information
type AgentInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
}
