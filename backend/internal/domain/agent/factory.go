package agent

import (
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/search"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/skill"
)

// Factory creates agents
type Factory struct {
	llmClient      *llm.OpenAIClient
	searcher       search.Searcher
	logger         *logger.Logger
	aShareRegistry *skill.Registry // skills for the A-share agent
	usStockRegistry *skill.Registry // skills for the US-stock agent
	cryptoRegistry  *skill.Registry // skills for the crypto agent
}

// NewAgentFactory creates a new agent factory.
// Each market gets its own skill registry so tools can be tailored per agent type.
func NewAgentFactory(
	llmClient *llm.OpenAIClient,
	searcher search.Searcher,
	logger *logger.Logger,
	aShareRegistry *skill.Registry,
	usStockRegistry *skill.Registry,
	cryptoRegistry *skill.Registry,
) *Factory {
	return &Factory{
		llmClient:       llmClient,
		searcher:        searcher,
		logger:          logger,
		aShareRegistry:  aShareRegistry,
		usStockRegistry: usStockRegistry,
		cryptoRegistry:  cryptoRegistry,
	}
}

// CreateAgent creates an agent by type
func (f *Factory) CreateAgent(agentType string) (Agent, error) {
	switch agentType {
	// Market agents (primary)
	case TypeAShare:
		return NewAShareAgent(f.llmClient, f.searcher, f.aShareRegistry, f.logger), nil
	case TypeUSStock:
		return NewUSStockAgent(f.llmClient, f.searcher, f.usStockRegistry, f.logger), nil
	case TypeCrypto:
		return NewCryptoAgent(f.llmClient, f.searcher, f.cryptoRegistry, f.logger), nil

	// Legacy agents (kept for backward compatibility)
	case TypeOrchestrator:
		return NewOrchestratorAgent(f.llmClient, f.logger), nil
	case TypeConversation:
		return NewConversationAgent(f.llmClient, f.logger), nil
	case TypeInvestmentAdvisor:
		return NewAShareAgent(f.llmClient, f.searcher, f.aShareRegistry, f.logger), nil
	case TypeTradingAgent, TypeTrading:
		return NewCryptoAgent(f.llmClient, f.searcher, f.cryptoRegistry, f.logger), nil

	default:
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
}

// GetAvailableAgents returns the three market modules available to users
func (f *Factory) GetAvailableAgents() []AgentInfo {
	return []AgentInfo{
		{
			Type:        TypeAShare,
			Name:        "A 股",
			Description: "沪深北交所 · A股个股分析、行业研究、政策解读",
			Icon:        "chart.bar.xaxis",
			Color:       "#C62828",
		},
		{
			Type:        TypeUSStock,
			Name:        "美 股",
			Description: "NYSE/NASDAQ · 美股研究、财报分析、宏观经济",
			Icon:        "dollarsign.circle",
			Color:       "#1565C0",
		},
		{
			Type:        TypeCrypto,
			Name:        "币 圈",
			Description: "加密货币 · 现货合约分析、链上数据、DeFi研究",
			Icon:        "bitcoinsign.circle",
			Color:       "#E65100",
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
