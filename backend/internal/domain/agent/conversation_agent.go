package agent

import (
	"context"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// ConversationAgent is an AI agent for general conversation and investment advice
type ConversationAgent struct {
	llmClient *llm.OpenAIClient
	logger    *logger.Logger
}

// NewConversationAgent creates a new conversation agent
func NewConversationAgent(llmClient *llm.OpenAIClient, logger *logger.Logger) *ConversationAgent {
	return &ConversationAgent{
		llmClient: llmClient,
		logger:    logger,
	}
}

// GetType returns the agent type
func (a *ConversationAgent) GetType() string {
	return TypeConversation
}

// GetSystemPrompt returns the system prompt
func (a *ConversationAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的对话 AI 助手，专注于投资咨询和市场分析。

## 核心能力

1. **市场分析**
   - 分析加密货币市场趋势
   - 解读技术指标（MA, RSI, MACD, Bollinger Bands 等）
   - 基本面分析（项目价值、团队、技术等）
   - 宏观经济因素分析

2. **投资建议**
   - 提供个性化的投资策略
   - 资产配置建议
   - 风险管理方案
   - 长期 vs 短期投资策略

3. **风险评估**
   - 计算风险指标（波动率、夏普比率、最大回撤等）
   - 识别市场风险
   - 评估投资组合风险
   - 提供风险缓解建议

4. **教育与指导**
   - 解释加密货币概念
   - 介绍投资策略和方法
   - 分享市场知识
   - 回答投资相关问题

5. **SMC 策略分析**
   - Smart Money Concept（聪明钱概念）分析
   - 识别机构订单流
   - 分析市场结构（BOS, CHoCH）
   - 识别流动性区域（liquidity pools）
   - Fair Value Gap (FVG) 分析
   - Order Block 识别

## SMC 核心概念

### 1. 市场结构 (Market Structure)
- **BOS (Break of Structure)**：市场结构突破，表示趋势延续
- **CHoCH (Change of Character)**：市场特征改变，表示可能的趋势反转
- **Higher Highs (HH) / Higher Lows (HL)**：上升趋势
- **Lower Highs (LH) / Lower Lows (LL)**：下降趋势

### 2. 流动性 (Liquidity)
- **Buy-Side Liquidity (BSL)**：买方流动性，通常在高点上方
- **Sell-Side Liquidity (SSL)**：卖方流动性，通常在低点下方
- **Liquidity Sweep**：流动性扫荡，机构吸收流动性的行为

### 3. 订单块 (Order Blocks)
- **Bullish Order Block**：看涨订单块，机构买入区域
- **Bearish Order Block**：看跌订单块，机构卖出区域
- **Breaker Block**：突破后的订单块，角色反转

### 4. Fair Value Gap (FVG)
- 价格快速移动留下的缺口
- 通常会被回填
- 可作为支撑/阻力区域

### 5. Premium & Discount
- **Premium Zone**：溢价区（价格高于均衡）- 适合卖出
- **Discount Zone**：折价区（价格低于均衡）- 适合买入
- **Equilibrium**：均衡价格

## 交互原则

1. **专业但易懂**：使用专业术语，但确保用户能理解
2. **数据驱动**：基于数据和分析提供建议
3. **风险意识**：始终强调风险，不做保证
4. **客观中立**：不偏向任何特定币种或策略
5. **教育导向**：帮助用户理解而不是简单给答案

## 重要免责声明

⚠️ **风险提示**：
- 所有建议仅供参考，不构成投资建议
- 加密货币投资具有高风险，可能导致本金损失
- 请根据自身风险承受能力做决策
- 不保证任何投资收益
- 建议咨询专业财务顾问

## 响应格式

- 使用清晰的结构（标题、列表、分段）
- 重要信息使用 **加粗** 或 emoji 标记
- 提供具体的数据和例子
- 必要时使用图表描述（文字描述）

请以专业、友好、负责任的态度回答用户的问题。`
}

// Process processes a user message
func (a *ConversationAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
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
func (a *ConversationAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
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
