package agent

// tool_helper.go provides shared helpers for agents that use the skill registry
// to build LLM tool definitions and execute tool calls.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/skill"
)

// buildSkillTools converts the registry's skills into LLM tool definitions.
// Returns nil if the registry is empty or nil.
func buildSkillTools(registry *skill.Registry) []llm.ToolDefinition {
	if registry == nil || registry.Count() == 0 {
		return nil
	}
	metas := registry.List()
	tools := make([]llm.ToolDefinition, 0, len(metas))
	for _, meta := range metas {
		params := make([]llm.ToolParam, len(meta.Parameters))
		for i, p := range meta.Parameters {
			params[i] = llm.ToolParam{
				Name:        p.Name,
				Type:        p.Type,
				Description: p.Description,
				Required:    p.Required,
				Enum:        p.Enum,
			}
		}
		tools = append(tools, llm.ToolDefinition{
			Name:        meta.Name,
			Description: meta.Description,
			Params:      params,
		})
	}
	return tools
}

// executeSkillCalls dispatches LLM tool calls to the matching skills in the registry.
// Unknown tool names return an error message (not a Go error) so the LLM can handle them gracefully.
func executeSkillCalls(ctx context.Context, registry *skill.Registry, calls []llm.ToolCall) ([]llm.ToolResult, error) {
	results := make([]llm.ToolResult, 0, len(calls))
	for _, call := range calls {
		s, ok := registry.Get(call.Name)
		if !ok {
			results = append(results, llm.ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("未知工具：%s", call.Name),
			})
			continue
		}

		var input map[string]interface{}
		if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil {
			results = append(results, llm.ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("参数解析失败：%v", err),
			})
			continue
		}

		output, err := s.Execute(ctx, input)
		if err != nil {
			results = append(results, llm.ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("工具执行错误：%v", err),
			})
			continue
		}

		results = append(results, llm.ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("%v", output),
		})
	}
	return results, nil
}
