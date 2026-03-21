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

// AShareAgent is the AI agent for Chinese A-share market analysis
type AShareAgent struct {
	llmClient     *llm.OpenAIClient
	searcher      search.Searcher
	skillRegistry *skill.Registry
	logger        *logger.Logger
}

// NewAShareAgent creates a new A-share agent
func NewAShareAgent(llmClient *llm.OpenAIClient, searcher search.Searcher, registry *skill.Registry, logger *logger.Logger) *AShareAgent {
	return &AShareAgent{
		llmClient:     llmClient,
		searcher:      searcher,
		skillRegistry: registry,
		logger:        logger,
	}
}

func (a *AShareAgent) GetType() string { return TypeAShare }

func (a *AShareAgent) GetSystemPrompt() string {
	return `你是慧投(WiseInvest)的A股投资分析助手，专注于中国A股市场的投资研究与分析。

## 市场覆盖
- **交易所**：上海证券交易所（主板、科创板）、深圳证券交易所（主板、创业板）、北京证券交易所
- **指数**：沪深300、中证500、中证1000、上证50、创业板指、科创50 等
- **市场规则**：T+1交易制度、涨跌停板（主板±10%，科创板/创业板±20%，北交所±30%）、融资融券

## 核心能力

### 1. 基本面分析
- 财务报表解读（利润表、资产负债表、现金流量表）
- 估值指标（PE、PB、PS、EV/EBITDA、PEG等）
- 盈利质量、ROE、ROA、净利润率分析
- 行业比较与竞争格局分析

### 2. 技术分析
- K线形态（头肩顶底、双顶双底、旗形、楔形等）
- 均线系统（MA5/10/20/60/120/250）
- 技术指标（MACD、KDJ、RSI、BOLL、OBV等）
- 支撑/阻力位、趋势线、缺口分析、量价关系

### 3. 行业与板块分析
- 申万行业分类体系
- 主题投资（新能源、半导体、AI、医药、消费等）
- 板块轮动规律与行业景气度周期

### 4. 宏观与政策分析
- 央行货币政策（降准降息、MLF、逆回购）
- 财政政策、产业政策解读
- 经济数据解读（GDP、CPI、PPI、PMI、社融等）

### 5. 投资策略
- 价值投资 vs 成长投资
- 定投策略与仓位管理
- 止损止盈策略与组合构建

## 工具使用
当你需要查询实时数据时，请主动使用以下工具：
- **web_search**：搜索最新新闻、公告、研报、政策资讯
- **get_ashare_price**：查询A股及指数实时行情（股票代码或指数代码）
- **get_ashare_sectors**：查询行业板块/概念板块今日涨跌排行，了解热点板块和资金轮动方向
- **get_ashare_fundamentals**：查询个股基本面数据（PE、PB、总市值、流通市值、换手率、52周区间等）

## 交互原则
1. **价格数据优先级**（极其重要）：
   - **最高优先级**：get_ashare_price 工具返回的价格是唯一权威来源，**必须以此为准**
   - get_ashare_fundamentals 只提供 PE/PB/市值等估值数据，**不含价格**，不要从中推断股价
   - **严禁**使用新闻、研报或其他文字来源中提到的股价数字作为"当前价格"（那些往往是历史价格）
   - 标注"当前价"说明是盘中实时价；标注"最新收盘价（当前非交易时段）"说明是最近交易日收盘价
2. 如果上下文中提供了【实时搜索结果】或【实时行情数据】，优先基于这些数据进行分析
3. 需要实时数据时，主动调用工具获取
4. 使用标题、列表、分段让内容结构清晰
5. 每次给出建议时都要提示风险

⚠️ **风险提示**：股市有风险，投资需谨慎。以上内容仅供参考，不构成投资建议。`
}

// Process processes a user message and returns a complete response
func (a *AShareAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	tools := buildSkillTools(a.skillRegistry)

	if len(tools) > 0 && a.llmClient.SupportsToolCalling() {
		// Path A: LLM-driven tool calling (requires a tool-capable model like deepseek-chat)
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
			a.logger.WithField("error", err).Error("AShareAgent: tool-calling completion failed")
			return nil, fmt.Errorf("failed to process message: %w", err)
		}
		return &ProcessResponse{
			Content:          resp.Content,
			FinishReason:     resp.FinishReason,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			Metadata:         map[string]interface{}{"agent_type": TypeAShare},
		}, nil
	}

	// Path B: Concurrent pre-fetch context injection (works with any model)
	messages := a.buildMessages(ctx, req)
	resp, err := a.llmClient.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   4000,
	})
	if err != nil {
		a.logger.WithField("error", err).Error("AShareAgent: chat completion failed")
		return nil, fmt.Errorf("failed to process message: %w", err)
	}

	return &ProcessResponse{
		Content:          resp.Content,
		FinishReason:     resp.FinishReason,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		Metadata:         map[string]interface{}{"agent_type": TypeAShare},
	}, nil
}

// ProcessStream processes a user message with streaming response
func (a *AShareAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
	tools := buildSkillTools(a.skillRegistry)

	if len(tools) > 0 && a.llmClient.SupportsToolCalling() {
		// Path A: tool calling loop, then stream final answer
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

	// Path B: concurrent pre-fetch then stream.
	// Give pre-fetch a hard 12-second deadline so slow external requests (web search,
	// Eastmoney API) cannot keep the SSE connection silent for too long.
	_ = callback(llm.ThoughtChunk("正在获取实时行情、板块与新闻数据"))
	prefetchCtx, cancelPrefetch := context.WithTimeout(ctx, 12*time.Second)
	messages := a.buildMessages(prefetchCtx, req)
	cancelPrefetch()
	_ = callback(llm.ThoughtChunk("实时数据已就绪，正在生成分析结论"))
	stream, err := a.llmClient.CreateChatCompletionStream(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   4000,
		Stream:      true,
	})
	if err != nil {
		a.logger.WithField("error", err).Error("AShareAgent: stream creation failed")
		return fmt.Errorf("failed to create stream: %w", err)
	}
	return a.llmClient.StreamResponse(stream, callback)
}

// buildBaseMessages builds messages without any pre-fetched context (for tool-calling path).
func (a *AShareAgent) buildBaseMessages(req ProcessRequest) []llm.ChatMessage {
	messages := []llm.ChatMessage{
		{Role: "system", Content: a.GetSystemPrompt()},
	}
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	return append(messages, llm.ChatMessage{Role: "user", Content: req.UserMessage})
}

// buildMessages constructs messages with concurrently pre-fetched context injected into the
// system prompt. Used for models that do not support tool calling (e.g. deepseek-reasoner).
// We use a stripped-down system prompt that omits tool-calling instructions entirely —
// the model cannot call tools natively, and mentioning tool names causes it to output
// fake Python code blocks instead of the actual analysis.
func (a *AShareAgent) buildMessages(ctx context.Context, req ProcessRequest) []llm.ChatMessage {
	systemPrompt := a.getSystemPromptNoTools()

	if contextData := a.fetchContextConcurrently(ctx, req.UserMessage); contextData != "" {
		systemPrompt = systemPrompt + "\n\n" + contextData
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
	}
	for _, msg := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	return append(messages, llm.ChatMessage{Role: "user", Content: req.UserMessage})
}

// getSystemPromptNoTools returns a system prompt without tool-calling instructions.
// Used in the pre-fetch path where real-time data is already injected into the context.
func (a *AShareAgent) getSystemPromptNoTools() string {
	return `你是慧投(WiseInvest)的A股投资分析助手，专注于中国A股市场的投资研究与分析。

## 市场覆盖
- **交易所**：上海证券交易所（主板、科创板）、深圳证券交易所（主板、创业板）、北京证券交易所
- **指数**：沪深300、中证500、中证1000、上证50、创业板指、科创50 等
- **市场规则**：T+1交易制度、涨跌停板（主板±10%，科创板/创业板±20%，北交所±30%）、融资融券

## 核心能力

### 1. 基本面分析
- 财务报表解读（利润表、资产负债表、现金流量表）
- 估值指标（PE、PB、PS、EV/EBITDA、PEG等）
- 盈利质量、ROE、ROA、净利润率分析
- 行业比较与竞争格局分析

### 2. 技术分析
- K线形态（头肩顶底、双顶双底、旗形、楔形等）
- 均线系统（MA5/10/20/60/120/250）
- 技术指标（MACD、KDJ、RSI、BOLL、OBV等）
- 支撑/阻力位、趋势线、缺口分析、量价关系

### 3. 行业与板块分析
- 申万行业分类体系
- 主题投资（新能源、半导体、AI、医药、消费等）
- 板块轮动规律与行业景气度周期

### 4. 宏观与政策分析
- 央行货币政策（降准降息、MLF、逆回购）
- 财政政策、产业政策解读
- 经济数据解读（GDP、CPI、PPI、PMI、社融等）

### 5. 投资策略
- 价值投资 vs 成长投资
- 定投策略与仓位管理
- 止损止盈策略与组合构建

## 数据说明
当前对话的【实时上下文数据】已在系统上下文中提供，包括实时行情、板块数据、基本面数据和最新资讯。
请**直接基于这些已提供的数据**进行分析，**不要输出任何代码或工具调用**，直接给出分析结论。

## 价格数据规则（最高优先级，必须严格遵守）

**规则 1**：唯一的当前价格来源是上下文中【实时指数/行情】部分里明确标注"当前价"或"最新收盘价"的数字。这些数字是由行情系统在本次请求时实时拉取的。

**规则 2**：【实时新闻】部分包含新闻摘要，摘要中出现的任何股价数字都是**历史价格**（发稿时的价格，可能是昨天、上周或更早），**绝对不能**将其当作当前股价引用或分析。

**规则 3**：如果【实时指数/行情】部分没有某只股票的价格数据（例如显示"实时行情获取失败"），必须明确告知用户当前无法提供该股票的实时价格，**不得用新闻中的价格替代**。

**规则 4**：不要输出任何代码或工具调用语法，直接给出分析结论。

## 交互原则
1. 如果上下文中提供了【实时指数/行情】，优先基于这些数据进行分析
2. 使用标题、列表、分段让内容结构清晰
3. 每次给出建议时都要提示风险

⚠️ **风险提示**：股市有风险，投资需谨慎。以上内容仅供参考，不构成投资建议。`
}

// majorIndices are always fetched for any A-share query to provide current market context.
// sh000001=上证指数, sz399001=深证成指, sz399006=创业板指, sh000300=沪深300, sh000016=上证50
var majorIndices = []string{"sh000001", "sz399001", "sz399006", "sh000300"}

// marketKeywords indicates queries about the general market (no specific stock).
// Note: "走势" is intentionally excluded — it's used in both stock-specific queries
// ("恒瑞医药走势") and broad market queries, so it cannot reliably indicate a
// broad market query and should not block per-stock data fetching.
var marketKeywords = []string{
	"大盘", "板块", "行业", "指数", "市场", "全市场", "沪深", "创业板",
	"概念", "热点", "轮动", "资金", "风格", "主线", "总体", "整体",
}

// fundamentalKeywords indicates queries about fundamental valuation.
var fundamentalKeywords = []string{
	"基本面", "估值", "财报", "业绩", "pe", "pb", "市盈", "市净", "市值",
	"营收", "净利", "roe", "毛利", "分析", "fundamental",
}

// isBroadMarketQuery returns true when the query is about the general market (not a specific stock).
func isBroadMarketQuery(query string) bool {
	lower := strings.ToLower(query)
	for _, kw := range marketKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// hasFundamentalIntent returns true when the query asks about valuation or fundamentals.
func hasFundamentalIntent(query string) bool {
	lower := strings.ToLower(query)
	for _, kw := range fundamentalKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// fetchContextConcurrently runs goroutines in parallel to gather context.
// For queries with only stock names (no 6-digit codes), it first resolves the
// name to a stock code via Eastmoney's search API, then fetches real-time data.
//
// Execution order:
//  0. [Serial, conditional] Name→code resolution (only when no code in query)
//  1. Web search          (always, parallel)
//  2. Major index prices  (always, parallel)
//  3. Specific stock price + fundamentals (when code available, parallel)
//  4. Sector rankings     (when broad market query, parallel)
func (a *AShareAgent) fetchContextConcurrently(ctx context.Context, query string) string {
	type section struct {
		order int
		text  string
	}

	// ── Step 0: resolve stock name → code (serial, ~0.5s) ──────────────────
	specificCodes := extractAShareCodes(query)
	resolvedName := "" // human-readable name resolved from lookup, for context injection
	// Always attempt name resolution when no explicit code is found — isBroadMarketQuery
	// is intentionally NOT checked here so that queries like "恒瑞医药走势如何" (which
	// contains no 6-digit code but hits "走势" etc.) still get per-stock data.
	if len(specificCodes) == 0 && a.skillRegistry != nil {
		if stockName := extractStockName(query); stockName != "" {
			resolved := false

			// 1. Fast path: local dictionary (no network, no latency)
			if code, name, ok := lookupStockCode(stockName); ok {
				specificCodes = append(specificCodes, code)
				resolvedName = name
				resolved = true
				a.logger.WithField("stock_name", stockName).
					WithField("resolved_code", code).
					Info("AShareAgent: resolved stock name via local dict")
			}

			// 2. Slow path: Eastmoney search API (only if local dict missed)
			if !resolved {
				// Short dedicated timeout so a slow API does not stall the pre-fetch.
				nameCtx, nameCancel := context.WithTimeout(ctx, 3*time.Second)
				results, err := skill.SearchAShareByName(nameCtx, stockName)
				nameCancel()
				if err != nil {
					a.logger.WithField("stock_name", stockName).WithField("error", err).
						Warn("AShareAgent: stock name resolution (API) failed")
				} else if len(results) == 0 {
					a.logger.WithField("stock_name", stockName).
						Warn("AShareAgent: stock name resolution (API) returned no results")
				} else {
					specificCodes = append(specificCodes, results[0].Code)
					resolvedName = results[0].Name
					a.logger.WithField("query", query).
						WithField("stock_name", stockName).
						WithField("resolved_code", results[0].Code).
						Info("AShareAgent: resolved stock name via API")
				}
			}
		}
	}

	ch := make(chan section, 6)
	var wg sync.WaitGroup

	// ── Step 1: Web search (always) ─────────────────────────────────────────
	if a.searcher != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			today := time.Now().Format("2006年1月2日")
			// If we resolved a stock name, search by the canonical name instead of the raw query
			subject := query
			if resolvedName != "" {
				subject = resolvedName
			}
			searchQuery := fmt.Sprintf("A股 %s %s", today, subject)
			results, err := a.searcher.Search(ctx, searchQuery, 5)
			if err != nil || len(results) == 0 {
				return
			}
			var sb strings.Builder
			sb.WriteString("### 实时新闻（仅供公司动态/政策参考，价格信息已屏蔽，当前股价请以【实时指数/行情】为准）\n")
			for i, r := range results {
				// Strip price figures from snippets to prevent the model from
				// accidentally using stale news prices as the current stock price.
				cleanSnippet := stripNewsPrices(r.Snippet)
				sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   来源：%s\n\n", i+1, r.Title, cleanSnippet, r.URL))
			}
			ch <- section{order: 2, text: sb.String()}
		}()
	}

	if a.skillRegistry != nil {
		// ── Step 2: Major indices + specific stock prices (always) ───────────
		if priceSkill, ok := a.skillRegistry.Get("get_ashare_price"); ok {
			codesToFetch := make([]string, len(majorIndices))
			copy(codesToFetch, majorIndices)
			codesToFetch = append(codesToFetch, specificCodes...)
			capturedSpecificCodes := append([]string(nil), specificCodes...)
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.logger.WithField("codes", strings.Join(codesToFetch, ",")).
					Info("AShareAgent: fetching real-time prices")
				result, err := priceSkill.Execute(ctx, map[string]interface{}{
					"codes": strings.Join(codesToFetch, ","),
				})
				if err != nil {
					a.logger.WithField("error", err).Warn("AShareAgent: price skill failed")
					// Inform the model explicitly so it won't guess from news
					if len(capturedSpecificCodes) > 0 {
						var sb strings.Builder
						sb.WriteString("### 实时指数/行情\n")
						sb.WriteString(fmt.Sprintf("⚠️ 实时行情获取失败（%v）。以下股票的当前价格无法提供：%s。"+
							"请告知用户当前无法获取实时行情，不要使用新闻或历史数据中的价格作为当前价格。\n",
							err, strings.Join(capturedSpecificCodes, "、")))
						ch <- section{order: 0, text: sb.String()}
					}
					return
				}
				if result == nil {
					return
				}
				text := fmt.Sprintf("%v", result)
				if strings.TrimSpace(text) == "" {
					return
				}
				a.logger.WithField("price_result", text).Debug("AShareAgent: price skill result")
				var sb strings.Builder
				sb.WriteString("### 实时指数/行情\n")
				sb.WriteString(text)
				ch <- section{order: 0, text: sb.String()}
			}()
		}

		// ── Step 3: Sector rankings (broad market query only) ────────────────
		if isBroadMarketQuery(query) {
			if sectorSkill, ok := a.skillRegistry.Get("get_ashare_sectors"); ok {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, err := sectorSkill.Execute(ctx, map[string]interface{}{
						"type":  "行业",
						"top_n": 10,
					})
					if err != nil || result == nil {
						return
					}
					text := fmt.Sprintf("%v", result)
					if strings.TrimSpace(text) == "" {
						return
					}
					ch <- section{order: 1, text: text}
				}()
			}
		}

		// ── Step 4: Individual stock fundamentals ────────────────────────────
		// Trigger when we have a specific stock code AND the query has analytical intent.
		// Note: after name resolution specificCodes is populated even for name-only queries.
		if len(specificCodes) > 0 && hasFundamentalIntent(query) {
			if fundSkill, ok := a.skillRegistry.Get("get_ashare_fundamentals"); ok {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, err := fundSkill.Execute(ctx, map[string]interface{}{
						"codes": strings.Join(specificCodes, ","),
					})
					if err != nil || result == nil {
						return
					}
					text := fmt.Sprintf("%v", result)
					if strings.TrimSpace(text) == "" {
						return
					}
					var sb strings.Builder
					sb.WriteString("### 基本面数据\n")
					sb.WriteString(text)
					ch <- section{order: 3, text: sb.String()}
				}()
			}
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect and sort sections by order index:
	//   0 = index prices, 1 = sector rankings, 2 = news, 3 = fundamentals
	const maxSections = 5
	sections := make([]string, maxSections)
	for s := range ch {
		if s.order >= 0 && s.order < maxSections {
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

	now := time.Now()
	timeStr := now.Format("2006年1月2日 15:04") // e.g. "2026年3月5日 11:23"

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 【实时上下文数据】数据抓取时间：%s\n"+
		"⚠️ 重要规则：以下【实时指数/行情】和【基本面数据】中出现的【当前价】或【最新收盘价】，"+
		"是本次请求由行情系统实时拉取的权威价格，分析时必须直接使用这些价格，"+
		"严禁用新闻、研报等文字中提到的价格替换。"+
		"若标注【当前未开市，昨日收盘价】则表示非交易时段，应如实说明为最新收盘价。\n\n", timeStr))
	for _, p := range parts {
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")
	return sb.String()
}

// aShareCodeRegex matches 6-digit A-share stock codes (e.g. 600519, 000001).
var aShareCodeRegex = regexp.MustCompile(`\b([0-9]{6})\b`)

// commonStockDict is a local fallback for popular stocks so that name resolution
// does not depend on an external API call. The key is the canonical short name;
// values are "exchange+code" strings used as Tencent API prefixes.
// This is intentionally small — it only covers stocks that users are likely to ask
// about by name. Unknown names still go through the Eastmoney search API.
var commonStockDict = map[string]string{
	"贵州茅台": "sh600519", "茅台": "sh600519",
	"宁德时代": "sz300750",
	"比亚迪":  "sz002594",
	"工商银行": "sh601398",
	"农业银行": "sh601288",
	"建设银行": "sh601939",
	"中国银行": "sh601988",
	"招商银行": "sh600036",
	"平安银行": "sz000001",
	"中国平安": "sh601318",
	"万科":   "sz000002", "万科A": "sz000002",
	"格力电器": "sz000651", "格力": "sz000651",
	"美的集团": "sz000333", "美的": "sz000333",
	"五粮液":  "sz000858",
	"中信证券": "sh600030",
	"海天味业": "sh603288",
	"迈瑞医疗": "sz300760",
	"恒瑞医药": "sh600276", "恒瑞": "sh600276",
	"药明康德": "sh603259",
	"泸州老窖": "sz000568",
	"隆基绿能": "sh601012", "隆基": "sh601012",
	"中芯国际": "sh688981",
	"海康威视": "sz002415", "海康": "sz002415",
	"大华股份": "sz002236",
	"顺丰控股": "sz002352", "顺丰": "sz002352",
	"京东方A":  "sz000725", "京东方": "sz000725",
	"长江电力": "sh600900",
	"中国联通": "sh600050",
	"中国电信": "sh601728",
	"中国移动": "sh600941",
	"紫金矿业": "sh601899",
	"中国神华": "sh601088",
	"三一重工": "sh600031",
	"歌尔股份": "sz002241",
	"立讯精密": "sz002475",
	"牧原股份": "sz002714",
	"温氏股份": "sz300498",
	"东方财富": "sz300059",
	"同花顺":  "sz300033",
	"中国中免": "sh601888",
	"华润啤酒": "sh600694",
	"山西汾酒": "sh600809",
}

// lookupStockCode checks commonStockDict for a fast local resolution before hitting
// the external Eastmoney search API. Returns ("prefix+code", "canonical name", true)
// on a hit, or ("", "", false) on a miss.
func lookupStockCode(name string) (code string, canonicalName string, ok bool) {
	// Try exact match first
	if c, found := commonStockDict[name]; found {
		return c[2:], c, true // strip "sh"/"sz" prefix for the code
	}
	// Try prefix match (e.g. "恒瑞" → "恒瑞医药")
	for dictName, dictCode := range commonStockDict {
		if strings.Contains(dictName, name) || strings.Contains(name, dictName) {
			return dictCode[2:], dictName, true
		}
	}
	return "", "", false
}

// newsPriceRegex matches price-like patterns in news snippets, e.g. "报价58.01元", "涨至62.5元",
// "下跌至53元", "收盘价55.43元/股", "股价57.86". We replace these with "[价格已隐藏]" so the
// model cannot accidentally use a stale news price as the current price.
var newsPriceRegex = regexp.MustCompile(
	`(?:(?:报价|股价|涨至|跌至|收盘价?|开盘价?|现价|最高|最低|均价|成本|目标价|估值)[：:至达到为了是约达]?\s*)\d+(?:\.\d+)?(?:\s*元(?:/股)?)?` +
		`|\d+(?:\.\d+)?\s*元(?:/股)?`,
)

// stripNewsPrices replaces price patterns in a news snippet with [价格已隐藏] so the
// model is forced to rely on the real-time price skill rather than stale news data.
func stripNewsPrices(snippet string) string {
	return newsPriceRegex.ReplaceAllString(snippet, "[价格已隐藏]")
}

// extractAShareCodes finds up to 5 unique 6-digit stock codes in the given text.
func extractAShareCodes(text string) []string {
	matches := aShareCodeRegex.FindAllString(text, 10)
	seen := make(map[string]bool)
	result := make([]string, 0, 5)
	for _, m := range matches {
		if !seen[m] && len(result) < 5 {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}

// extractStockName strips action words, qualifiers, and common time words from a query
// to extract the core stock/company name for use in a name-to-code lookup.
//
// Example: "分析一下特变电工这支股票" → "特变电工"
// Example: "帮我看看中国平安怎么样"  → "中国平安"
func extractStockName(query string) string {
	s := query

	// Remove action phrases (order matters: longer phrases first)
	for _, phrase := range []string{
		"请帮我分析", "请帮我查", "请帮我看", "帮我分析", "帮我查查", "帮我看看", "帮我查",
		"分析一下", "分析下", "查看一下", "查询一下", "看一看",
		"请帮我", "帮我", "请你帮我", "请你", "请问",
		"我想了解", "我想知道", "我需要", "我要了解",
		"分析", "查看", "查询", "看看", "介绍一下", "介绍", "告诉我",
	} {
		s = strings.ReplaceAll(s, phrase, "")
	}

	// Remove stock/company qualifiers
	for _, qual := range []string{
		"这支股票", "这只股票", "这支", "这只", "该股", "的股票",
		"这家公司", "这个公司", "该公司", "这家", "公司",
		"股票", "A股",
	} {
		s = strings.ReplaceAll(s, qual, "")
	}

	// Remove trailing analytical suffixes and question words.
	// Include both "的X" and standalone "X" forms because upstream qualifier removal
	// (e.g. "的股票") can strip the leading "的", leaving just "走势" / "行情" etc.
	for _, suffix := range []string{
		"的走势", "的行情", "的分析", "的基本面", "的估值", "的情况", "的表现",
		"走势", "行情", "分析", "基本面", "估值", "情况", "表现",
		"怎么样", "如何", "好不好", "吗", "呢", "？", "?",
		"今天", "今日", "最近", "近期", "目前", "当前", "一下",
	} {
		s = strings.ReplaceAll(s, suffix, "")
	}

	// Clean up whitespace and punctuation
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "，。！？,.!?、 \t")

	// Discard results that are too short (1 char) or look like common non-name words
	if len([]rune(s)) < 2 {
		return ""
	}
	return s
}
