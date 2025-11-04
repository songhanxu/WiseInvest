package agent

import (
	"context"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// InvestmentAdvisorAgent is an AI agent for investment advice
type InvestmentAdvisorAgent struct {
	llmClient *llm.OpenAIClient
	logger    *logger.Logger
}

// NewInvestmentAdvisorAgent creates a new investment advisor agent
func NewInvestmentAdvisorAgent(llmClient *llm.OpenAIClient, logger *logger.Logger) *InvestmentAdvisorAgent {
	return &InvestmentAdvisorAgent{
		llmClient: llmClient,
		logger:    logger,
	}
}

// GetType returns the agent type
func (a *InvestmentAdvisorAgent) GetType() string {
	return TypeInvestmentAdvisor
}

// GetSystemPrompt returns the system prompt
func (a *InvestmentAdvisorAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的专业投资顾问AI助手。你的职责是：

## 核心能力
1. **市场分析**：分析加密货币市场趋势、技术指标和基本面
2. **风险评估**：评估投资风险，计算波动率、夏普比率等关键指标
3. **投资建议**：提供个性化的投资建议和资产配置方案
4. **情绪管理**：识别投资者情绪，提供理性的决策支持

## 交互原则
- 使用专业但易懂的语言
- 提供数据支持的分析
- 强调风险提示
- 保持客观中立
- 鼓励理性投资

## 重要提醒
- 所有建议仅供参考，不构成投资建议
- 加密货币投资具有高风险
- 建议用户根据自身情况做决策
- 不保证任何投资收益

请以专业、友好、负责任的态度回答用户的投资问题。`
}

// Process processes a user message
func (a *InvestmentAdvisorAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	// Build messages for LLM
	messages := []llm.ChatMessage{
		{
			Role:    "system",
			Content: a.GetSystemPrompt(),
		},
	}

	// Add conversation history
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add current user message
	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: req.UserMessage,
	})

	// Call LLM
	resp, err := a.llmClient.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
	})
	if err != nil {
		a.logger.WithField("error", err).Error("Failed to create chat completion")
		return nil, fmt.Errorf("failed to process message: %w", err)
	}

	return &ProcessResponse{
		Content:          resp.Content,
		FinishReason:     resp.FinishReason,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		Metadata: map[string]interface{}{
			"agent_type": a.GetType(),
		},
	}, nil
}

// ProcessStream processes a user message with streaming
func (a *InvestmentAdvisorAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
	// Build messages for LLM
	messages := []llm.ChatMessage{
		{
			Role:    "system",
			Content: a.GetSystemPrompt(),
		},
	}

	// Add conversation history
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add current user message
	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: req.UserMessage,
	})

	// Create streaming request
	stream, err := a.llmClient.CreateChatCompletionStream(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
		Stream:      true,
	})
	if err != nil {
		a.logger.WithField("error", err).Error("Failed to create chat completion stream")
		return fmt.Errorf("failed to create stream: %w", err)
	}

	// Stream response
	return a.llmClient.StreamResponse(stream, callback)
}
