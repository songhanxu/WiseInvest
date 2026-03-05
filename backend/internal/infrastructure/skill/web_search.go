package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/search"
)

// WebSearchSkill searches the web for real-time financial information.
// It wraps the existing search.Searcher so agents can call web search as an LLM tool.
type WebSearchSkill struct {
	searcher    search.Searcher
	queryPrefix string // market-specific prefix, e.g. "A股 " or "crypto "
}

// NewWebSearchSkill creates a WebSearchSkill.
// queryPrefix is prepended to every search query (e.g. "A股 " for A-share agents).
func NewWebSearchSkill(searcher search.Searcher, queryPrefix string) *WebSearchSkill {
	return &WebSearchSkill{searcher: searcher, queryPrefix: strings.TrimSpace(queryPrefix)}
}

func (s *WebSearchSkill) Name() string { return "web_search" }

func (s *WebSearchSkill) Description() string {
	return "搜索互联网，获取最新的股票新闻、公司公告、财报信息、市场动态、政策资讯等实时内容。当需要了解近期发生的事件时使用此工具。"
}

func (s *WebSearchSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "query",
			Type:        "string",
			Description: "搜索关键词，例如：'茅台2024三季报'、'美联储利率决议'、'比特币减半行情'",
			Required:    true,
		},
		{
			Name:        "num_results",
			Type:        "integer",
			Description: "返回结果数量，范围 1-10，默认为 5",
			Required:    false,
		},
	}
}

func (s *WebSearchSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	numResults := 5
	if n, ok := input["num_results"]; ok {
		switch v := n.(type) {
		case float64:
			numResults = int(v)
		case int:
			numResults = v
		}
	}
	if numResults < 1 {
		numResults = 1
	}
	if numResults > 10 {
		numResults = 10
	}

	fullQuery := query
	if s.queryPrefix != "" {
		fullQuery = s.queryPrefix + " " + query
	}

	results, err := s.searcher.Search(ctx, fullQuery, numResults)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		return "未找到相关搜索结果，请尝试调整关键词。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索词：「%s」\n\n", fullQuery))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   来源：%s\n\n", i+1, r.Title, r.Snippet, r.URL))
	}
	return sb.String(), nil
}
