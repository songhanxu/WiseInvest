package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/search"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/skill"
)

// USStockAgent is the AI agent for US stock market analysis
type USStockAgent struct {
	llmClient     *llm.OpenAIClient
	searcher      search.Searcher
	skillRegistry *skill.Registry
	logger        *logger.Logger
}

func NewUSStockAgent(llmClient *llm.OpenAIClient, searcher search.Searcher, registry *skill.Registry, logger *logger.Logger) *USStockAgent {
	return &USStockAgent{llmClient: llmClient, searcher: searcher, skillRegistry: registry, logger: logger}
}

func (a *USStockAgent) GetType() string { return TypeUSStock }

func (a *USStockAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的美股投资分析助手，专注于美国股票市场的投资研究与分析。

## 市场覆盖
- **交易所**：纽约证券交易所(NYSE)、纳斯达克(NASDAQ)、美国证券交易所(AMEX)
- **指数**：S&P 500、纳斯达克100、道琼斯工业指数、罗素2000
- **市场规则**：T+2交割、无涨跌停限制、盘前盘后交易

## 核心能力
- 美股基本面分析（EPS、P/E、P/S、EV/EBITDA等）
- 科技股、成长股分析（FAANG、AI概念等）
- 财报季分析与预期差
- 宏观经济（美联储政策、通胀、就业数据）
- 期权策略基础（Covered Call、Put 保护等）
- ADR与中概股分析

## 工具使用
当你需要查询实时数据时，请主动使用以下工具：
- **web_search**：搜索最新新闻、财报、分析师报告
- **get_us_stock_price**：查询美股实时行情（需要股票代码如 AAPL、NVDA）

⚠️ **风险提示**：美股投资还涉及汇率风险、时差操作风险，请充分了解后谨慎决策。`
}

func (a *USStockAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	tools := buildSkillTools(a.skillRegistry)

	if len(tools) > 0 && a.llmClient.SupportsToolCalling() {
		messages := a.buildBaseMessages(req)
		resp, err := a.llmClient.CreateChatCompletionWithToolLoop(
			ctx,
			llm.ChatCompletionRequest{Messages: messages, Temperature: 0.7, MaxTokens: 4000},
			tools,
			func(c context.Context, calls []llm.ToolCall) ([]llm.ToolResult, error) {
				return executeSkillCalls(c, a.skillRegistry, calls)
			},
			5,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to process message: %w", err)
		}
		return &ProcessResponse{
			Content: resp.Content, FinishReason: resp.FinishReason,
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens: resp.Usage.TotalTokens,
			Metadata:    map[string]interface{}{"agent_type": TypeUSStock},
		}, nil
	}

	messages := a.buildMessages(ctx, req)
	resp, err := a.llmClient.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: messages, Temperature: 0.7, MaxTokens: 4000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to process message: %w", err)
	}
	return &ProcessResponse{
		Content: resp.Content, FinishReason: resp.FinishReason,
		PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens: resp.Usage.TotalTokens,
		Metadata:    map[string]interface{}{"agent_type": TypeUSStock},
	}, nil
}

func (a *USStockAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
	tools := buildSkillTools(a.skillRegistry)

	if len(tools) > 0 && a.llmClient.SupportsToolCalling() {
		messages := a.buildBaseMessages(req)
		return a.llmClient.StreamChatCompletionWithToolLoop(
			ctx,
			llm.ChatCompletionRequest{Messages: messages, Temperature: 0.7, MaxTokens: 4000},
			tools,
			func(c context.Context, calls []llm.ToolCall) ([]llm.ToolResult, error) {
				return executeSkillCalls(c, a.skillRegistry, calls)
			},
			5,
			callback,
		)
	}

	_ = callback(llm.ThoughtChunk("正在获取美股实时行情与相关新闻"))
	messages := a.buildMessages(ctx, req)
	_ = callback(llm.ThoughtChunk("实时数据已就绪，正在生成分析结论"))
	stream, err := a.llmClient.CreateChatCompletionStream(ctx, llm.ChatCompletionRequest{
		Messages: messages, Temperature: 0.7, MaxTokens: 4000, Stream: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	return a.llmClient.StreamResponse(stream, callback)
}

func (a *USStockAgent) buildBaseMessages(req ProcessRequest) []llm.ChatMessage {
	messages := []llm.ChatMessage{{Role: "system", Content: a.GetSystemPrompt()}}
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	return append(messages, llm.ChatMessage{Role: "user", Content: req.UserMessage})
}

func (a *USStockAgent) buildMessages(ctx context.Context, req ProcessRequest) []llm.ChatMessage {
	systemPrompt := a.GetSystemPrompt()

	if contextData := a.fetchContextConcurrently(ctx, req.UserMessage); contextData != "" {
		systemPrompt = systemPrompt + "\n\n" + contextData
	}

	messages := []llm.ChatMessage{{Role: "system", Content: systemPrompt}}
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	return append(messages, llm.ChatMessage{Role: "user", Content: req.UserMessage})
}

// fetchContextConcurrently concurrently fetches web search and (if tickers detected) stock prices.
func (a *USStockAgent) fetchContextConcurrently(ctx context.Context, query string) string {
	type section struct {
		order int
		text  string
	}

	ch := make(chan section, 3)
	var wg sync.WaitGroup

	// 1. Web search — include today's date to surface same-day articles
	if a.searcher != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			today := time.Now().Format("January 2 2006")
			results, err := a.searcher.Search(ctx, fmt.Sprintf("%s %s", today, query), 5)
			if err != nil || len(results) == 0 {
				return
			}
			var sb strings.Builder
			sb.WriteString("### Real-time News\n")
			for i, r := range results {
				sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   Source: %s\n\n", i+1, r.Title, r.Snippet, r.URL))
			}
			ch <- section{order: 1, text: sb.String()}
		}()
	}

	// 2. US stock price (if uppercase tickers detected)
	if tickers := extractUSTickers(query); len(tickers) > 0 && a.skillRegistry != nil {
		if priceSkill, ok := a.skillRegistry.Get("get_us_stock_price"); ok {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := priceSkill.Execute(ctx, map[string]interface{}{
					"symbols": strings.Join(tickers, ","),
				})
				if err != nil || result == nil {
					return
				}
				text := fmt.Sprintf("%v", result)
				if strings.TrimSpace(text) == "" {
					return
				}
				var sb strings.Builder
				sb.WriteString("### Real-time Quote\n")
				sb.WriteString(text)
				ch <- section{order: 0, text: sb.String()}
			}()
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	sections := make([]string, 3)
	for s := range ch {
		if s.order < len(sections) {
			sections[s.order] = s.text
		}
	}

	var parts []string
	for _, s := range sections {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}

	timeStr := time.Now().Format("January 2, 2006 15:04 MST")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 【Real-time Context】Data fetched at: %s\nUse this as current data. Do NOT refer to it as \"yesterday\" unless the source explicitly says so:\n\n", timeStr))
	for _, p := range parts {
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")
	return sb.String()
}

// Common non-ticker all-caps words to filter out from ticker detection.
var nonTickerWords = map[string]bool{
	"AI": false, "API": true, "US": true, "UK": true, "EU": true,
	"IPO": true, "ETF": true, "CEO": true, "CFO": true, "COO": true,
	"GDP": true, "CPI": true, "PPI": true, "PMI": true, "FED": true,
	"EPS": true, "PE": true, "PB": true, "ROE": true, "ROA": true,
}

// usTickerRegex matches 1-5 uppercase ASCII letters as potential stock tickers.
var usTickerRegex = regexp.MustCompile(`\b([A-Z]{1,5})\b`)

// extractUSTickers finds likely US stock ticker symbols in the text.
func extractUSTickers(text string) []string {
	matches := usTickerRegex.FindAllString(text, 20)
	seen := make(map[string]bool)
	result := make([]string, 0, 5)
	for _, m := range matches {
		if nonTickerWords[m] {
			continue
		}
		if len(m) < 2 {
			continue
		}
		if !seen[m] && len(result) < 5 {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}
