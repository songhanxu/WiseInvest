package llm

import (
	"context"
	"fmt"
	"strings"
)

type ChatSelection struct {
	Model  string
	Effort string
}

type chatSelectionKey struct{}

func WithChatSelection(ctx context.Context, selection ChatSelection) context.Context {
	if selection.Model == "" {
		return ctx
	}
	return context.WithValue(ctx, chatSelectionKey{}, selection)
}

func chatSelection(ctx context.Context) ChatSelection {
	selection, _ := ctx.Value(chatSelectionKey{}).(ChatSelection)
	return selection
}

// ValidateChatSelection restricts user choices to the official DeepSeek models.
// Empty fields preserve existing clients' default behavior.
func (c *OpenAIClient) ValidateChatSelection(model, effort string) (ChatSelection, error) {
	model = strings.TrimSpace(model)
	effort = strings.TrimSpace(effort)
	if model == "" && effort == "" {
		return ChatSelection{}, nil
	}
	if !c.deepSeekEndpoint {
		return ChatSelection{}, fmt.Errorf("chat model selection requires the official DeepSeek endpoint")
	}
	if model == "" {
		model = c.model
	}
	if model != "deepseek-flash" && (model != "deepseek-v4-pro" || c.ProModel() != "deepseek-v4-pro") {
		return ChatSelection{}, fmt.Errorf("unsupported chat model")
	}
	if effort == "" {
		effort = "none"
	}
	switch effort {
	case "none", "low", "high", "max":
		return ChatSelection{Model: model, Effort: effort}, nil
	default:
		return ChatSelection{}, fmt.Errorf("unsupported reasoning effort")
	}
}

func (c *OpenAIClient) requestOptions(ctx context.Context, requestedModel string, maxTokens int, mode, effort string) (context.Context, string, int) {
	model := c.model
	if requestedModel != "" {
		model = requestedModel
	}
	if selected := chatSelection(ctx); selected.Model != "" {
		model = selected.Model
		if selected.Effort == "none" {
			mode, effort = "disabled", ""
		} else {
			mode, effort = "enabled", selected.Effort
			switch selected.Effort {
			case "low":
				maxTokens = max(maxTokens, 8192)
			case "high":
				maxTokens = max(maxTokens, 16384)
			case "max":
				maxTokens = max(maxTokens, 32768)
			}
		}
	}
	if c.SupportsThinkingControl(model) {
		if mode == "disabled" || mode == "enabled" {
			ctx = context.WithValue(ctx, thinkingModeKey{}, mode)
		}
		if effort == "low" || effort == "high" || effort == "max" {
			ctx = context.WithValue(ctx, reasoningEffortKey{}, effort)
		}
	}
	return ctx, model, maxTokens
}
