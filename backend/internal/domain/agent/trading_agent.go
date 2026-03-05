package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/binance"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// TradingAgent is an AI agent for trading operations with Binance API and SMC strategy
type TradingAgent struct {
	llmClient     *llm.OpenAIClient
	logger        *logger.Logger
	binanceClient *binance.Client
	smcStrategy   *binance.SMCStrategy
}

// NewTradingAgent creates a new trading agent
func NewTradingAgent(llmClient *llm.OpenAIClient, logger *logger.Logger) *TradingAgent {
	// Note: Binance credentials should be loaded from config/env
	// For now, we'll initialize with empty credentials (will be set later)
	binanceClient := binance.NewClient("", "")
	smcStrategy := binance.NewSMCStrategy(binanceClient)
	
	return &TradingAgent{
		llmClient:     llmClient,
		logger:        logger,
		binanceClient: binanceClient,
		smcStrategy:   smcStrategy,
	}
}

// SetBinanceCredentials sets Binance API credentials
func (a *TradingAgent) SetBinanceCredentials(apiKey, apiSecret string) {
	a.binanceClient = binance.NewClient(apiKey, apiSecret)
	a.smcStrategy = binance.NewSMCStrategy(a.binanceClient)
}

// GetType returns the agent type
func (a *TradingAgent) GetType() string {
	return TypeTradingAgent
}

// GetSystemPrompt returns the system prompt
func (a *TradingAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的智能交易助手，集成了币安 API 和 SMC（Smart Money Concept）交易策略。

## 核心能力

### 1. 交易执行（基于 SMC 策略）
- 执行加密货币交易操作
- 基于 SMC 分析自动生成交易信号
- 智能止损止盈设置
- 多目标位管理

### 2. 账户管理
- 查询账户余额
- 查看持仓情况
- 订单历史查询
- 资金流水分析

### 3. SMC 市场分析
- **市场结构分析**：识别 BOS、CHoCH、趋势方向
- **订单块识别**：找出机构订单区域
- **Fair Value Gap**：识别价格缺口
- **流动性分析**：识别流动性池和扫荡区域
- **Premium/Discount 区域**：判断价格位置

### 4. 风险管理
- 基于 ATR 的动态止损
- 多目标位分批止盈
- 仓位管理建议
- 风险回报比计算

## 可用的工具函数

你可以调用以下函数来执行操作（通过 JSON 格式返回）：

### 账户相关
- 'get_account_info': 获取账户信息
- 'get_balance': 获取特定资产余额

### 市场分析
- 'analyze_market_smc': 执行 SMC 市场分析
- 'generate_trade_signal': 基于 SMC 生成交易信号
- 'get_market_data': 获取市场数据（K线、价格等）

### 交易操作
- 'create_order': 创建订单
- 'cancel_order': 取消订单
- 'get_open_orders': 获取未完成订单

## 响应格式

当需要执行操作时，返回 JSON 格式：

{
  "action": "function_name",
  "parameters": {
    "param1": "value1",
    "param2": "value2"
  },
  "explanation": "向用户解释你要做什么",
  "requires_confirmation": true/false
}

## SMC 交易规则

### 买入条件
1. 市场结构为看涨（Higher Highs, Higher Lows）
2. 价格在 Discount Zone（折价区）
3. 价格接近 Bullish Order Block
4. 可选：存在未填补的 Bullish FVG

### 卖出条件
1. 市场结构为看跌（Lower Highs, Lower Lows）
2. 价格在 Premium Zone（溢价区）
3. 价格接近 Bearish Order Block
4. 可选：存在未填补的 Bearish FVG

### 止损设置
- 买入：Order Block 下方 0.5-1 ATR
- 卖出：Order Block 上方 0.5-1 ATR

### 止盈设置
- TP1: 1.5R（风险的 1.5 倍）
- TP2: 2.5R
- TP3: 4R（让利润奔跑）

## 安全规则

⚠️ **重要**：
1. 所有交易操作必须经过用户明确确认
2. 清晰说明风险和潜在损失
3. 不执行超出用户风险承受能力的交易
4. 异常市场情况及时提醒
5. 保护用户资金安全是第一优先级

## 交互原则

1. **透明度**：清晰解释 SMC 分析结果
2. **教育性**：帮助用户理解 SMC 概念
3. **谨慎性**：强调风险，不做收益保证
4. **确认机制**：重要操作需要二次确认
5. **实时反馈**：及时更新执行状态

请以专业、谨慎、负责任的态度协助用户进行交易操作。`
}

// TradingAction represents a trading action
type TradingAction struct {
	Action               string                 `json:"action"`
	Parameters           map[string]interface{} `json:"parameters"`
	Explanation          string                 `json:"explanation"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
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
		Temperature: 0.3, // Lower temperature for more precise trading instructions
		MaxTokens:   2000,
	})
	if err != nil {
		a.logger.WithField("error", err).Error("Failed to create chat completion")
		return nil, fmt.Errorf("failed to process message: %w", err)
	}

	// Try to parse action from response
	content := resp.Content
	action, err := a.parseAction(content)
	if err == nil && action != nil {
		// Execute the action
		result, err := a.executeAction(ctx, action)
		if err != nil {
			content = fmt.Sprintf("%s\n\n❌ 执行失败：%s", content, err.Error())
		} else {
			content = fmt.Sprintf("%s\n\n✅ 执行结果：\n%s", content, result)
		}
	}

	return &ProcessResponse{
		Content:          content,
		FinishReason:     resp.FinishReason,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		Metadata: map[string]interface{}{
			"agent_type": a.GetType(),
			"action":     action,
		},
	}, nil
}

// parseAction parses trading action from LLM response
func (a *TradingAgent) parseAction(content string) (*TradingAction, error) {
	// Look for JSON code block
	if !strings.Contains(content, "```json") && !strings.Contains(content, "{") {
		return nil, fmt.Errorf("no action found")
	}

	jsonStr := content
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end > 0 {
			jsonStr = content[start : start+end]
		}
	} else if strings.Contains(content, "```") {
		start := strings.Index(content, "```") + 3
		end := strings.Index(content[start:], "```")
		if end > 0 {
			jsonStr = content[start : start+end]
		}
	} else {
		// Try to extract JSON object
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			jsonStr = content[start : end+1]
		}
	}

	var action TradingAction
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonStr)), &action); err != nil {
		return nil, err
	}

	return &action, nil
}

// executeAction executes a trading action
func (a *TradingAgent) executeAction(ctx context.Context, action *TradingAction) (string, error) {
	a.logger.WithField("action", action.Action).Info("Executing trading action")

	switch action.Action {
	case "get_account_info":
		return a.getAccountInfo(ctx)
	case "analyze_market_smc":
		symbol := action.Parameters["symbol"].(string)
		return a.analyzeMarketSMC(ctx, symbol)
	case "generate_trade_signal":
		symbol := action.Parameters["symbol"].(string)
		return a.generateTradeSignal(ctx, symbol)
	case "get_market_data":
		symbol := action.Parameters["symbol"].(string)
		return a.getMarketData(ctx, symbol)
	case "create_order":
		return a.createOrder(ctx, action.Parameters)
	case "get_open_orders":
		symbol := ""
		if s, ok := action.Parameters["symbol"].(string); ok {
			symbol = s
		}
		return a.getOpenOrders(ctx, symbol)
	case "cancel_order":
		symbol := action.Parameters["symbol"].(string)
		orderID := int64(action.Parameters["order_id"].(float64))
		return a.cancelOrder(ctx, symbol, orderID)
	default:
		return "", fmt.Errorf("unknown action: %s", action.Action)
	}
}

// getAccountInfo gets account information
func (a *TradingAgent) getAccountInfo(ctx context.Context) (string, error) {
	info, err := a.binanceClient.GetAccountInfo(ctx)
	if err != nil {
		return "", err
	}

	result := "📊 **账户信息**\n\n"
	result += fmt.Sprintf("账户类型：%s\n", info.AccountType)
	result += fmt.Sprintf("可交易：%v\n", info.CanTrade)
	result += "\n**余额**：\n"

	for _, balance := range info.Balances {
		if balance.Free != "0.00000000" || balance.Locked != "0.00000000" {
			result += fmt.Sprintf("- %s: 可用 %s, 冻结 %s\n", balance.Asset, balance.Free, balance.Locked)
		}
	}

	return result, nil
}

// analyzeMarketSMC performs SMC analysis
func (a *TradingAgent) analyzeMarketSMC(ctx context.Context, symbol string) (string, error) {
	analysis, err := a.smcStrategy.AnalyzeMarket(ctx, symbol)
	if err != nil {
		return "", err
	}

	result := fmt.Sprintf("📈 **%s SMC 分析**\n\n", symbol)
	result += fmt.Sprintf("**市场结构**：%s\n", analysis.MarketStructure.Trend)
	result += fmt.Sprintf("**均衡价格**：%.2f\n\n", analysis.Equilibrium)

	if analysis.PremiumZone != nil {
		result += fmt.Sprintf("**溢价区 (Premium)**：%.2f - %.2f\n", analysis.PremiumZone.Low, analysis.PremiumZone.High)
	}
	if analysis.DiscountZone != nil {
		result += fmt.Sprintf("**折价区 (Discount)**：%.2f - %.2f\n\n", analysis.DiscountZone.Low, analysis.DiscountZone.High)
	}

	result += fmt.Sprintf("**订单块数量**：%d\n", len(analysis.OrderBlocks))
	if len(analysis.OrderBlocks) > 0 {
		result += "最近的订单块：\n"
		for i, ob := range analysis.OrderBlocks {
			if i >= 3 {
				break
			}
			result += fmt.Sprintf("- %s OB: %.2f - %.2f (强度: %.2f)\n", ob.Type, ob.Low, ob.High, ob.Strength)
		}
	}

	result += fmt.Sprintf("\n**Fair Value Gaps**：%d 个\n", len(analysis.FairValueGaps))
	result += fmt.Sprintf("**流动性区域**：%d 个\n\n", len(analysis.LiquidityZones))

	result += fmt.Sprintf("**建议**：%s\n", analysis.Recommendation)
	result += fmt.Sprintf("**置信度**：%.0f%%\n", analysis.Confidence*100)

	return result, nil
}

// generateTradeSignal generates trade signal
func (a *TradingAgent) generateTradeSignal(ctx context.Context, symbol string) (string, error) {
	analysis, err := a.smcStrategy.AnalyzeMarket(ctx, symbol)
	if err != nil {
		return "", err
	}

	signal, err := a.smcStrategy.GenerateTradeSignal(ctx, symbol, analysis)
	if err != nil {
		return "", err
	}

	result := fmt.Sprintf("🎯 **%s 交易信号**\n\n", symbol)
	result += fmt.Sprintf("**操作**：%s\n", signal.Action)

	if signal.Action != "HOLD" {
		result += fmt.Sprintf("**入场价**：%.2f\n", signal.EntryPrice)
		result += fmt.Sprintf("**止损**：%.2f\n", signal.StopLoss)
		result += fmt.Sprintf("**止盈目标**：\n")
		result += fmt.Sprintf("  - TP1: %.2f\n", signal.TakeProfit1)
		result += fmt.Sprintf("  - TP2: %.2f\n", signal.TakeProfit2)
		result += fmt.Sprintf("  - TP3: %.2f\n", signal.TakeProfit3)
		result += fmt.Sprintf("**风险回报比**：1:%.2f\n", signal.RiskReward)
		result += fmt.Sprintf("**置信度**：%.0f%%\n\n", signal.Confidence*100)
		result += fmt.Sprintf("**理由**：%s\n", signal.Reasoning)
	} else {
		result += "\n当前没有明确的交易机会，建议等待更好的入场点。\n"
	}

	return result, nil
}

// getMarketData gets market data
func (a *TradingAgent) getMarketData(ctx context.Context, symbol string) (string, error) {
	ticker, err := a.binanceClient.Get24hrTicker(ctx, symbol)
	if err != nil {
		return "", err
	}

	result := fmt.Sprintf("📊 **%s 市场数据**\n\n", symbol)
	result += fmt.Sprintf("**最新价格**：%s\n", ticker.LastPrice)
	result += fmt.Sprintf("**24h 涨跌**：%s (%s%%)\n", ticker.PriceChange, ticker.PriceChangePercent)
	result += fmt.Sprintf("**24h 最高**：%s\n", ticker.HighPrice)
	result += fmt.Sprintf("**24h 最低**：%s\n", ticker.LowPrice)
	result += fmt.Sprintf("**24h 成交量**：%s\n", ticker.Volume)
	result += fmt.Sprintf("**24h 成交额**：%s\n", ticker.QuoteVolume)

	return result, nil
}

// createOrder creates an order
func (a *TradingAgent) createOrder(ctx context.Context, params map[string]interface{}) (string, error) {
	// This is a placeholder - actual implementation would need proper parameter validation
	return "⚠️ 订单创建功能需要用户明确确认。请确认交易参数后再执行。", nil
}

// getOpenOrders gets open orders
func (a *TradingAgent) getOpenOrders(ctx context.Context, symbol string) (string, error) {
	orders, err := a.binanceClient.GetOpenOrders(ctx, symbol)
	if err != nil {
		return "", err
	}

	result := "📋 **未完成订单**\n\n"
	if len(orders) == 0 {
		result += "当前没有未完成的订单。\n"
	} else {
		for _, order := range orders {
			result += fmt.Sprintf("- %s %s %s\n", order.Symbol, order.Side, order.Type)
			result += fmt.Sprintf("  价格: %s, 数量: %s, 状态: %s\n", order.Price, order.OrigQty, order.Status)
		}
	}

	return result, nil
}

// cancelOrder cancels an order
func (a *TradingAgent) cancelOrder(ctx context.Context, symbol string, orderID int64) (string, error) {
	order, err := a.binanceClient.CancelOrder(ctx, symbol, orderID)
	if err != nil {
		return "", err
	}

	result := fmt.Sprintf("✅ 订单已取消\n\n")
	result += fmt.Sprintf("订单ID：%d\n", order.OrderID)
	result += fmt.Sprintf("交易对：%s\n", order.Symbol)
	result += fmt.Sprintf("状态：%s\n", order.Status)

	return result, nil
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
