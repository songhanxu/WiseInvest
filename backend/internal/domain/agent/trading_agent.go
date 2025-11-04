package agent

import (
	"context"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// TradingAgent is an AI agent for trading operations
type TradingAgent struct {
	llmClient *llm.OpenAIClient
	logger    *logger.Logger
}

// NewTradingAgent creates a new trading agent
func NewTradingAgent(llmClient *llm.OpenAIClient, logger *logger.Logger) *TradingAgent {
	return &TradingAgent{
		llmClient: llmClient,
		logger:    logger,
	}
}

// GetType returns the agent type
func (a *TradingAgent) GetType() string {
	return TypeTradingAgent
}

// GetSystemPrompt returns the system prompt
func (a *TradingAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的智能交易助手。你的职责是：

## 核心能力
1. **交易执行**：帮助用户执行加密货币交易操作
2. **订单管理**：创建、查询、取消订单
3. **账户查询**：查询账户余额、持仓情况
4. **风险控制**：设置止损止盈、仓位管理

## 交互原则
- 清晰确认交易参数
- 重要操作需要二次确认
- 实时反馈执行状态
- 强调风险提示

## 安全规则
- 所有交易操作必须经过用户明确确认
- 不执行超出用户风险承受能力的交易
- 及时提醒异常市场情况
- 保护用户资金安全

## 当前状态
注意：当前为演示版本，币安API集成功能正在开发中。
你可以模拟交易流程，但实际交易功能尚未启用。

请以专业、谨慎、负责任的态度协助用户进行交易操作。`
}

// Process processes a user message
func (a *TradingAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
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
		Temperature: 0.5, // Lower temperature for more precise trading instructions
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
func (a *TradingAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
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
		Temperature: 0.5,
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
