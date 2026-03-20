package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/search"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/skill"
)

// CryptoAgent is the AI agent for cryptocurrency market analysis
type CryptoAgent struct {
	llmClient     *llm.OpenAIClient
	searcher      search.Searcher
	skillRegistry *skill.Registry
	logger        *logger.Logger
}

func NewCryptoAgent(llmClient *llm.OpenAIClient, searcher search.Searcher, registry *skill.Registry, logger *logger.Logger) *CryptoAgent {
	return &CryptoAgent{llmClient: llmClient, searcher: searcher, skillRegistry: registry, logger: logger}
}

func (a *CryptoAgent) GetType() string { return TypeCrypto }

func (a *CryptoAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的加密货币投资分析助手，专注于数字资产市场的研究与分析。

## 市场覆盖
- **交易所**：Binance、OKX、Bybit、Coinbase 等主流交易所
- **资产类型**：比特币(BTC)、以太坊(ETH)、主流山寨币、DeFi代币、Layer2等
- **交易类型**：现货、合约（永续/交割）、期权

## 核心能力
- 链上数据分析（持仓分布、交易所净流量、巨鲸动向）
- 衍生品市场（资金费率、未平仓合约、期权偏斜度）
- DeFi/Web3 项目分析（TVL、协议收入、Token经济模型）
- SMC策略（BOS/CHoCH/OB/FVG/流动性分析）
- 宏观加密周期（减半周期、比特币主导率、Fear & Greed指数）
- 风险管理（合约杠杆、爆仓价计算、仓位管理）

## 工具使用
当你需要查询实时数据时，请主动使用以下工具：
- **web_search**：搜索最新加密新闻、项目动态、链上数据分析
- **get_crypto_price**：查询加密货币实时价格和24h涨跌幅

⚠️ **风险提示**：加密货币波动极大，合约交易可能导致本金全部损失，请严格控制仓位和杠杆。`
}

func (a *CryptoAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
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
			Metadata:    map[string]interface{}{"agent_type": TypeCrypto},
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
		Metadata:    map[string]interface{}{"agent_type": TypeCrypto},
	}, nil
}

func (a *CryptoAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
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

	_ = callback(llm.ThoughtChunk("正在获取加密市场实时行情与新闻数据"))
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

func (a *CryptoAgent) buildBaseMessages(req ProcessRequest) []llm.ChatMessage {
	messages := []llm.ChatMessage{{Role: "system", Content: a.GetSystemPrompt()}}
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	return append(messages, llm.ChatMessage{Role: "user", Content: req.UserMessage})
}

func (a *CryptoAgent) buildMessages(ctx context.Context, req ProcessRequest) []llm.ChatMessage {
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

// majorCryptoCoins are always fetched for any crypto query to provide current market context.
var majorCryptoCoins = []string{"btc", "eth", "bnb", "sol"}

// fetchContextConcurrently fetches web search and live crypto prices in parallel.
//
// Prices fetched:
//   - Major coins (always): BTC, ETH, BNB, SOL
//   - Specific coins: any crypto symbols detected in the user's query
func (a *CryptoAgent) fetchContextConcurrently(ctx context.Context, query string) string {
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
			today := time.Now().Format("2006年1月2日")
			results, err := a.searcher.Search(ctx, fmt.Sprintf("crypto %s %s", today, query), 5)
			if err != nil || len(results) == 0 {
				return
			}
			var sb strings.Builder
			sb.WriteString("### 实时新闻\n")
			for i, r := range results {
				sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   来源：%s\n\n", i+1, r.Title, r.Snippet, r.URL))
			}
			ch <- section{order: 1, text: sb.String()}
		}()
	}

	// 2. Crypto price — always fetch major coins, plus any specific coins in the query
	if a.skillRegistry != nil {
		if priceSkill, ok := a.skillRegistry.Get("get_crypto_price"); ok {
			// Merge major coins with any coins detected in the query
			seen := make(map[string]bool)
			coinsToFetch := make([]string, 0, len(majorCryptoCoins)+4)
			for _, c := range majorCryptoCoins {
				if !seen[c] {
					seen[c] = true
					coinsToFetch = append(coinsToFetch, c)
				}
			}
			for _, c := range extractCryptoSymbols(query) {
				if !seen[c] {
					seen[c] = true
					coinsToFetch = append(coinsToFetch, c)
				}
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := priceSkill.Execute(ctx, map[string]interface{}{
					"coins": strings.Join(coinsToFetch, ","),
				})
				if err != nil || result == nil {
					return
				}
				text := fmt.Sprintf("%v", result)
				if strings.TrimSpace(text) == "" {
					return
				}
				var sb strings.Builder
				sb.WriteString("### 实时行情\n")
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

	timeStr := time.Now().Format("2006年1月2日 15:04")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 【实时上下文数据】数据抓取时间：%s\n以下数据为本次请求实时获取，请以此为准，不要把当前数据描述为昨天或上一个交易日的数据（除非数据来源本身注明了日期）：\n\n", timeStr))
	for _, p := range parts {
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")
	return sb.String()
}

// knownCryptoSymbols is the set of recognised ticker symbols (lowercase).
var knownCryptoSymbols = map[string]string{
	"btc": "btc", "bitcoin": "btc",
	"eth": "eth", "ethereum": "eth",
	"bnb": "bnb",
	"sol": "sol", "solana": "sol",
	"xrp": "xrp", "ripple": "xrp",
	"ada": "ada", "cardano": "ada",
	"doge": "doge", "dogecoin": "doge",
	"avax": "avax",
	"dot": "dot", "polkadot": "dot",
	"link": "link", "chainlink": "link",
	"matic": "matic", "polygon": "matic",
	"ltc": "ltc", "litecoin": "ltc",
	"atom": "atom", "cosmos": "atom",
	"near": "near",
	"trx": "trx", "tron": "trx",
	"bch": "bch",
	"sui": "sui",
	"apt": "apt", "aptos": "apt",
	"arb": "arb", "arbitrum": "arb",
	"op": "op", "optimism": "op",
	"ton": "ton",
	"shib": "shib",
	"pepe": "pepe",
}

// extractCryptoSymbols scans text for known crypto ticker symbols/names.
func extractCryptoSymbols(text string) []string {
	lower := strings.ToLower(text)
	seen := make(map[string]bool)
	result := make([]string, 0, 5)

	for token, canonical := range knownCryptoSymbols {
		if strings.Contains(lower, token) && !seen[canonical] && len(result) < 6 {
			seen[canonical] = true
			result = append(result, canonical)
		}
	}
	return result
}
