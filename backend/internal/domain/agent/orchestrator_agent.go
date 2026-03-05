package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// OrchestratorAgent is the main agent that coordinates other agents
type OrchestratorAgent struct {
	llmClient         *llm.OpenAIClient
	logger            *logger.Logger
	conversationAgent *ConversationAgent
	tradingAgent      *TradingAgent
}

// NewOrchestratorAgent creates a new orchestrator agent
func NewOrchestratorAgent(llmClient *llm.OpenAIClient, logger *logger.Logger) *OrchestratorAgent {
	return &OrchestratorAgent{
		llmClient:         llmClient,
		logger:            logger,
		conversationAgent: NewConversationAgent(llmClient, logger),
		tradingAgent:      NewTradingAgent(llmClient, logger),
	}
}

// GetType returns the agent type
func (a *OrchestratorAgent) GetType() string {
	return TypeOrchestrator
}

// GetSystemPrompt returns the system prompt
func (a *OrchestratorAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的主控 AI 助手，负责协调和调度其他专业 Agent。

## 你的职责

1. **理解用户意图**：分析用户的请求，判断需要哪个 Agent 来处理
2. **任务分发**：将任务分配给合适的 Agent
3. **结果整合**：整合各个 Agent 的响应，提供统一的用户体验
4. **流程控制**：管理复杂的多步骤操作流程

## 可用的 Agent

### 1. Conversation Agent（对话 Agent）
- **用途**：处理一般性对话、投资咨询、市场分析
- **能力**：
  - 市场趋势分析
  - 投资建议
  - 风险评估
  - 教育性内容
  - 一般性问答

### 2. Trading Agent（交易 Agent）
- **用途**：执行实际的交易操作
- **能力**：
  - 币安账户查询
  - 下单交易（基于 SMC 策略）
  - 订单管理
  - 持仓查询
  - 风险控制

## 决策规则

**使用 Conversation Agent 的场景**：
- 用户询问市场观点、分析
- 请求投资建议或策略
- 学习加密货币知识
- 一般性聊天

**使用 Trading Agent 的场景**：
- 明确要求执行交易
- 查询账户信息
- 管理订单
- 设置止损止盈
- 任何涉及币安 API 的操作

**需要协调两个 Agent 的场景**：
- 先分析后交易
- 需要确认的复杂操作
- 多步骤流程

## 响应格式

你需要以 JSON 格式返回决策：

{
  "agent": "conversation|trading|orchestrator",
  "action": "具体的操作类型",
  "reasoning": "选择该 Agent 的原因",
  "parameters": {
    // 传递给目标 Agent 的参数
  },
  "user_message": "转发给目标 Agent 的用户消息（可能经过改写）"
}

## 重要原则

1. **安全第一**：涉及交易的操作必须明确用户意图
2. **透明度**：让用户知道哪个 Agent 在处理他们的请求
3. **确认机制**：重要操作需要用户确认
4. **错误处理**：优雅地处理 Agent 失败的情况

请根据用户的输入，做出最合适的决策。`
}

// AgentDecision represents the orchestrator's decision
type AgentDecision struct {
	Agent       string                 `json:"agent"`        // conversation, trading, orchestrator
	Action      string                 `json:"action"`       // 具体操作类型
	Reasoning   string                 `json:"reasoning"`    // 决策理由
	Parameters  map[string]interface{} `json:"parameters"`   // 参数
	UserMessage string                 `json:"user_message"` // 转发的消息
}

// Process processes a user message
func (a *OrchestratorAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	// Step 1: Analyze user intent and make decision
	decision, err := a.analyzeIntent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze intent: %w", err)
	}

	a.logger.WithField("decision", decision).Info("Orchestrator decision made")

	// Step 2: Route to appropriate agent
	var response *ProcessResponse
	switch decision.Agent {
	case "conversation":
		response, err = a.routeToConversationAgent(ctx, req, decision)
	case "trading":
		response, err = a.routeToTradingAgent(ctx, req, decision)
	case "orchestrator":
		// Handle directly
		response, err = a.handleDirectly(ctx, req, decision)
	default:
		return nil, fmt.Errorf("unknown agent type in decision: %s", decision.Agent)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to route to agent: %w", err)
	}

	// Step 3: Add orchestrator metadata
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}
	response.Metadata["orchestrator_decision"] = decision
	response.Metadata["routed_to"] = decision.Agent

	return response, nil
}

// ProcessStream processes a user message with streaming
func (a *OrchestratorAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
	// Step 1: Analyze user intent
	decision, err := a.analyzeIntent(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to analyze intent: %w", err)
	}

	a.logger.WithField("decision", decision).Info("Orchestrator decision made (stream)")

	// Step 2: Route to appropriate agent with streaming
	switch decision.Agent {
	case "conversation":
		return a.streamToConversationAgent(ctx, req, decision, callback)
	case "trading":
		return a.streamToTradingAgent(ctx, req, decision, callback)
	case "orchestrator":
		return a.streamDirectly(ctx, req, decision, callback)
	default:
		return fmt.Errorf("unknown agent type in decision: %s", decision.Agent)
	}
}

// analyzeIntent analyzes user intent and makes routing decision
func (a *OrchestratorAgent) analyzeIntent(ctx context.Context, req ProcessRequest) (*AgentDecision, error) {
	messages := []llm.ChatMessage{
		{
			Role:    "system",
			Content: a.GetSystemPrompt(),
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("用户消息：%s\n\n请分析用户意图并返回 JSON 格式的决策。", req.UserMessage),
		},
	}

	resp, err := a.llmClient.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.3, // Lower temperature for more consistent decisions
		MaxTokens:   500,
	})
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var decision AgentDecision
	content := strings.TrimSpace(resp.Content)
	
	// Extract JSON from markdown code blocks if present
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.LastIndex(content, "```")
		if end > start {
			content = content[start:end]
		}
	} else if strings.Contains(content, "```") {
		start := strings.Index(content, "```") + 3
		end := strings.LastIndex(content, "```")
		if end > start {
			content = content[start:end]
		}
	}

	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		a.logger.WithField("content", content).Warn("Failed to parse decision JSON, using fallback")
		// Fallback: route to conversation agent
		decision = AgentDecision{
			Agent:       "conversation",
			Action:      "general_conversation",
			Reasoning:   "Failed to parse orchestrator decision, defaulting to conversation agent",
			UserMessage: req.UserMessage,
			Parameters:  make(map[string]interface{}),
		}
	}

	return &decision, nil
}

// routeToConversationAgent routes request to conversation agent
func (a *OrchestratorAgent) routeToConversationAgent(ctx context.Context, req ProcessRequest, decision *AgentDecision) (*ProcessResponse, error) {
	// Modify request with decision parameters
	modifiedReq := req
	modifiedReq.UserMessage = decision.UserMessage
	if modifiedReq.Context == nil {
		modifiedReq.Context = make(map[string]interface{})
	}
	modifiedReq.Context["orchestrator_decision"] = decision

	return a.conversationAgent.Process(ctx, modifiedReq)
}

// routeToTradingAgent routes request to trading agent
func (a *OrchestratorAgent) routeToTradingAgent(ctx context.Context, req ProcessRequest, decision *AgentDecision) (*ProcessResponse, error) {
	modifiedReq := req
	modifiedReq.UserMessage = decision.UserMessage
	if modifiedReq.Context == nil {
		modifiedReq.Context = make(map[string]interface{})
	}
	modifiedReq.Context["orchestrator_decision"] = decision

	return a.tradingAgent.Process(ctx, modifiedReq)
}

// handleDirectly handles request directly by orchestrator
func (a *OrchestratorAgent) handleDirectly(ctx context.Context, req ProcessRequest, decision *AgentDecision) (*ProcessResponse, error) {
	// Build messages for LLM
	messages := []llm.ChatMessage{
		{
			Role:    "system",
			Content: "你是慧投的主控助手，负责协调和提供综合性的帮助。",
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

	resp, err := a.llmClient.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
	})
	if err != nil {
		return nil, err
	}

	return &ProcessResponse{
		Content:          resp.Content,
		FinishReason:     resp.FinishReason,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		Metadata: map[string]interface{}{
			"agent_type": a.GetType(),
			"decision":   decision,
		},
	}, nil
}

// streamToConversationAgent streams to conversation agent
func (a *OrchestratorAgent) streamToConversationAgent(ctx context.Context, req ProcessRequest, decision *AgentDecision, callback func(string) error) error {
	modifiedReq := req
	modifiedReq.UserMessage = decision.UserMessage
	if modifiedReq.Context == nil {
		modifiedReq.Context = make(map[string]interface{})
	}
	modifiedReq.Context["orchestrator_decision"] = decision

	return a.conversationAgent.ProcessStream(ctx, modifiedReq, callback)
}

// streamToTradingAgent streams to trading agent
func (a *OrchestratorAgent) streamToTradingAgent(ctx context.Context, req ProcessRequest, decision *AgentDecision, callback func(string) error) error {
	modifiedReq := req
	modifiedReq.UserMessage = decision.UserMessage
	if modifiedReq.Context == nil {
		modifiedReq.Context = make(map[string]interface{})
	}
	modifiedReq.Context["orchestrator_decision"] = decision

	return a.tradingAgent.ProcessStream(ctx, modifiedReq, callback)
}

// streamDirectly streams directly from orchestrator
func (a *OrchestratorAgent) streamDirectly(ctx context.Context, req ProcessRequest, decision *AgentDecision, callback func(string) error) error {
	messages := []llm.ChatMessage{
		{
			Role:    "system",
			Content: "你是慧投的主控助手，负责协调和提供综合性的帮助。",
		},
	}

	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	messages = append(messages, llm.ChatMessage{
		Role:    "user",
		Content: req.UserMessage,
	})

	stream, err := a.llmClient.CreateChatCompletionStream(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
		Stream:      true,
	})
	if err != nil {
		return err
	}

	return a.llmClient.StreamResponse(stream, callback)
}
