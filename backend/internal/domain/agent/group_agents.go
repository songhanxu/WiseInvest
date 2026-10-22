package agent

// group_agents.go implements the four persona agents that participate in the
// 慧投圆桌 (WiseInvest Roundtable) group chat.  Each agent has a distinct
// trading philosophy and uses the combined skill registry to fetch real-time
// data before forming an opinion.

import (
	"context"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/skill"
)

// Group agent persona IDs – these are the short IDs used in SSE events and
// carried by the iOS client to attribute each message to the right avatar.
const (
	GroupAgentIDOrchestrator = "orchestrator"
	GroupAgentIDValue        = "value"
	GroupAgentIDTrend        = "trend"
	GroupAgentIDQuant        = "quant"
)

// groupAgentOrder is the canonical discussion order used when all agents participate.
var groupAgentOrder = []string{
	GroupAgentIDOrchestrator,
	GroupAgentIDValue,
	GroupAgentIDTrend,
	GroupAgentIDQuant,
}

// GroupPersonaAgent is an AI agent with a specific investment personality.
// It implements the Agent interface and can use any skill in its registry.
type GroupPersonaAgent struct {
	agentID  string
	llm      *llm.OpenAIClient
	registry *skill.Registry
	logger   *logger.Logger
}

// NewGroupPersonaAgent creates a persona agent for the given ID.
func NewGroupPersonaAgent(agentID string, llmClient *llm.OpenAIClient, registry *skill.Registry, log *logger.Logger) *GroupPersonaAgent {
	return &GroupPersonaAgent{
		agentID:  agentID,
		llm:      llmClient,
		registry: registry,
		logger:   log,
	}
}

// AgentID returns the short persona ID (e.g. "value", "trend").
func (g *GroupPersonaAgent) AgentID() string { return g.agentID }

func (g *GroupPersonaAgent) GetType() string        { return TypeGroupChat + "_" + g.agentID }
func (g *GroupPersonaAgent) GetSystemPrompt() string { return groupPersonaSystemPrompt(g.agentID) }

// Process implements Agent (non-streaming).
func (g *GroupPersonaAgent) Process(ctx context.Context, req ProcessRequest) (*ProcessResponse, error) {
	tools := buildSkillTools(g.registry)
	messages := g.buildMessages(req)

	if len(tools) > 0 && g.llm.SupportsToolCalling() {
		resp, err := g.llm.CreateChatCompletionWithToolLoop(
			ctx,
			llm.ChatCompletionRequest{Messages: messages, Temperature: 0.7, MaxTokens: 2000},
			tools,
			func(c context.Context, calls []llm.ToolCall) ([]llm.ToolResult, error) {
				return executeSkillCalls(c, g.registry, calls)
			},
			5,
		)
		if err != nil {
			return nil, fmt.Errorf("group agent %s: %w", g.agentID, err)
		}
		return &ProcessResponse{Content: resp.Content}, nil
	}

	resp, err := g.llm.CreateChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
	})
	if err != nil {
		return nil, fmt.Errorf("group agent %s: %w", g.agentID, err)
	}
	return &ProcessResponse{Content: resp.Content}, nil
}

// ProcessStream implements Agent (streaming).
func (g *GroupPersonaAgent) ProcessStream(ctx context.Context, req ProcessRequest, callback func(string) error) error {
	tools := buildSkillTools(g.registry)
	messages := g.buildMessages(req)

	if len(tools) > 0 && g.llm.SupportsToolCalling() {
		return g.llm.StreamChatCompletionWithToolLoop(
			ctx,
			llm.ChatCompletionRequest{Messages: messages, Temperature: 0.7, MaxTokens: 2000},
			tools,
			func(c context.Context, calls []llm.ToolCall) ([]llm.ToolResult, error) {
				return executeSkillCalls(c, g.registry, calls)
			},
			5,
			callback,
		)
	}

	stream, err := g.llm.CreateChatCompletionStream(ctx, llm.ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
		Stream:      true,
	})
	if err != nil {
		return fmt.Errorf("group agent %s: %w", g.agentID, err)
	}
	return g.llm.StreamResponse(stream, callback)
}

// buildMessages constructs the LLM message array.
// Prior agents' responses in the same turn are appended to the user message
// so each agent is aware of what its peers have already said.
func (g *GroupPersonaAgent) buildMessages(req ProcessRequest) []llm.ChatMessage {
	messages := []llm.ChatMessage{{Role: "system", Content: g.GetSystemPrompt()}}

	for _, h := range req.ConversationHistory {
		messages = append(messages, llm.ChatMessage{Role: h.Role, Content: h.Content})
	}

	userContent := req.UserMessage
	if priorRaw, ok := req.Context["prior_responses"]; ok {
		if priorList, ok := priorRaw.([]string); ok && len(priorList) > 0 {
			userContent += "\n\n---\n**圆桌其他分析师的观点（请结合自己的独特视角发表见解，避免重复他人内容）：**\n\n"
			for _, p := range priorList {
				userContent += p + "\n\n"
			}
		}
	}

	messages = append(messages, llm.ChatMessage{Role: "user", Content: userContent})
	return messages
}

// GroupPersonaDisplayName returns the Chinese display name for a persona ID.
func GroupPersonaDisplayName(agentID string) string {
	switch agentID {
	case GroupAgentIDOrchestrator:
		return "主持人"
	case GroupAgentIDValue:
		return "价值派"
	case GroupAgentIDTrend:
		return "趋势派"
	case GroupAgentIDQuant:
		return "量化派"
	default:
		return agentID
	}
}

// groupPersonaSystemPrompt returns the system prompt for each trading persona.
func groupPersonaSystemPrompt(agentID string) string {
	toolsNote := `
**可用工具**：web_search（实时搜索）、get_ashare_price（A股行情）、get_us_stock_price（美股行情）、get_crypto_price（加密货币行情）、get_ashare_sectors（板块涨跌）、get_ashare_fundamentals（个股基本面）、lookup_ashare_code（查股票代码）。
主动调用相关工具获取实时数据后再作分析。回答简洁有力，200-400字为宜，突出你的分析视角。如有其他分析师观点，可以呼应或提出不同看法。`

	switch agentID {
	case GroupAgentIDOrchestrator:
		return `你是"慧投圆桌"的主持人，负责引导讨论、综合各方观点，给出平衡客观的结论。

**风格**：综合全局。你不偏向任何单一流派，而是从基本面、技术面、量化三个维度帮助用户全面理解投资机会与风险。先简要点评问题核心，引导各方分析师发言，最后给出你的综合判断与行动建议。
**口头禅**："综合来看"、"从多个维度"、"平衡风险与机会"。` + toolsNote

	case GroupAgentIDValue:
		return `你是"慧投圆桌"的价值派分析师，坚守格雷厄姆-巴菲特式价值投资理念。

**风格**：专注基本面与内在价值。只有当价格显著低于内在价值时才值得买入。你重点看护城河、现金流质量、ROE稳定性、负债率、分红历史。用PE、PB、PEG、EV/EBITDA判断安全边际。对短期波动漠然置之，追求5-10年维度的复利增长。
**口头禅**："安全边际"、"内在价值"、"护城河"、"长期复利"、"时间是好公司的朋友"。` + toolsNote

	case GroupAgentIDTrend:
		return `你是"慧投圆桌"的趋势派分析师，奉行"顺势而为"的交易哲学。

**风格**：专注价格行为与技术面。通过K线形态、均线系统、成交量、动量指标判断趋势方向和力度。重视右侧交易——等趋势确认再进场，不抄底不摸顶。关注放量突破、均线多头排列、MACD金叉等买入信号；跌破支撑、成交量萎缩则谨慎。
**口头禅**："趋势是朋友"、"放量突破"、"右侧进场"、"止损出局"、"量价配合"。` + toolsNote

	case GroupAgentIDQuant:
		return `你是"慧投圆桌"的量化派分析师，以数据和概率为信仰。

**风格**：纯数据驱动。用统计方法量化风险与收益——历史波动率、VaR、最大回撤、夏普比率、因子暴露。评估当前仓位的风险敞口，提出基于Kelly公式或均值-方差优化的仓位建议。不相信"感觉"，只看数字和模型，但也警惕过度拟合。
**口头禅**："统计显著"、"因子暴露"、"风险调整后收益"、"夏普比率"、"回撤控制"。` + toolsNote

	default:
		return `你是慧投圆桌的投资分析师，从专业角度为用户提供投资见解。` + toolsNote
	}
}

// OrderedParticipants filters and orders the given participant IDs using the
// canonical discussion order.  If participants is empty, all four are returned.
func OrderedParticipants(participants []string) []string {
	if len(participants) == 0 {
		return groupAgentOrder
	}
	set := make(map[string]bool, len(participants))
	for _, p := range participants {
		set[p] = true
	}
	out := make([]string, 0, len(participants))
	for _, id := range groupAgentOrder {
		if set[id] {
			out = append(out, id)
		}
	}
	return out
}
