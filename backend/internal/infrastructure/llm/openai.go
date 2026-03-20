package llm

import (
	"context"
	"fmt"
	"io"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

// OpenAIClient wraps the OpenAI client
type OpenAIClient struct {
	client *openai.Client
	model  string
}

const thoughtChunkPrefix = "__THOUGHT__:"

// ThoughtChunk wraps a backend thought/status line so upper layers can route it
// to the thinking panel instead of the final answer text.
func ThoughtChunk(content string) string {
	return thoughtChunkPrefix + content
}

// ParseThoughtChunk checks whether a stream chunk is a thought/status payload.
func ParseThoughtChunk(chunk string) (content string, ok bool) {
	if strings.HasPrefix(chunk, thoughtChunkPrefix) {
		return strings.TrimPrefix(chunk, thoughtChunkPrefix), true
	}
	return "", false
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(cfg config.OpenAIConfig) *OpenAIClient {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}

	return &OpenAIClient{
		client: openai.NewClientWithConfig(clientConfig),
		model:  cfg.Model,
	}
}

// SupportsToolCalling returns true when the configured model supports OpenAI-style function calling.
// Models like deepseek-reasoner and o1* do not support tool use.
func (c *OpenAIClient) SupportsToolCalling() bool {
	noToolModels := []string{"deepseek-reasoner", "o1-preview", "o1-mini", "o1"}
	for _, m := range noToolModels {
		if strings.Contains(c.model, m) {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────
// Basic types
// ─────────────────────────────────────────

// ChatMessage represents a chat message.
// ToolCalls and ToolCallID are only populated for tool-related messages.
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall // non-empty when Role=="assistant" with pending tool calls
	ToolCallID string     // non-empty when Role=="tool" (result of a tool call)
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Messages    []ChatMessage
	Temperature float32
	MaxTokens   int
	Stream      bool
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	Content      string
	FinishReason string
	Usage        Usage
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ─────────────────────────────────────────
// Tool calling types
// ─────────────────────────────────────────

// ToolParam describes a single parameter in a tool definition (JSON Schema format).
type ToolParam struct {
	Name        string
	Type        string // "string", "number", "integer", "boolean"
	Description string
	Required    bool
	Enum        []string
}

// ToolDefinition defines a callable function tool for the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	Params      []ToolParam
}

// ToolCall is a tool invocation requested by the LLM in its response.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON-encoded arguments
}

// ToolResult is the output of executing a single ToolCall.
type ToolResult struct {
	CallID  string
	Content string
}

// ToolCallHandler executes a batch of tool calls and returns their results.
type ToolCallHandler func(ctx context.Context, calls []ToolCall) ([]ToolResult, error)

// ─────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────

// toOpenAIMessage converts a ChatMessage (potentially carrying tool call info) to the go-openai type.
func toOpenAIMessage(msg ChatMessage) openai.ChatCompletionMessage {
	m := openai.ChatCompletionMessage{
		Role:       msg.Role,
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}
	if len(msg.ToolCalls) > 0 {
		m.ToolCalls = make([]openai.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			m.ToolCalls[i] = openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			}
		}
	}
	return m
}

// toOpenAITools converts our ToolDefinition slice to the go-openai format.
func toOpenAITools(tools []ToolDefinition) []openai.Tool {
	result := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		properties := make(map[string]interface{})
		required := []string{}
		for _, p := range t.Params {
			prop := map[string]interface{}{
				"type":        p.Type,
				"description": p.Description,
			}
			if len(p.Enum) > 0 {
				prop["enum"] = p.Enum
			}
			properties[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		result = append(result, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			},
		})
	}
	return result
}

// ─────────────────────────────────────────
// Standard completions (unchanged behaviour)
// ─────────────────────────────────────────

// CreateChatCompletion creates a chat completion
func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = toOpenAIMessage(msg)
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from OpenAI")
	}

	return &ChatCompletionResponse{
		Content:      resp.Choices[0].Message.Content,
		FinishReason: string(resp.Choices[0].FinishReason),
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

// CreateChatCompletionStream creates a streaming chat completion
func (c *OpenAIClient) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = toOpenAIMessage(msg)
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion stream: %w", err)
	}

	return stream, nil
}

// StreamResponse handles streaming response
func (c *OpenAIClient) StreamResponse(stream *openai.ChatCompletionStream, callback func(string) error) error {
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("stream error: %w", err)
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			if content != "" {
				if err := callback(content); err != nil {
					return err
				}
			}
		}
	}
}

// ─────────────────────────────────────────
// Tool calling loop
// ─────────────────────────────────────────

// CreateChatCompletionWithToolLoop runs a ReAct-style tool-calling loop and returns the final response.
// The loop continues until the model stops requesting tool calls or maxSteps is reached.
// maxSteps ≤ 0 defaults to 5.
func (c *OpenAIClient) CreateChatCompletionWithToolLoop(
	ctx context.Context,
	req ChatCompletionRequest,
	tools []ToolDefinition,
	handler ToolCallHandler,
	maxSteps int,
) (*ChatCompletionResponse, error) {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	openaiTools := toOpenAITools(tools)
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = toOpenAIMessage(msg)
	}

	for step := 0; step < maxSteps; step++ {
		apiReq := openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
		}
		if len(openaiTools) > 0 {
			apiReq.Tools = openaiTools
		}

		resp, err := c.client.CreateChatCompletion(ctx, apiReq)
		if err != nil {
			return nil, fmt.Errorf("chat completion failed (step %d): %w", step+1, err)
		}
		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no choices returned (step %d)", step+1)
		}

		choice := resp.Choices[0]

		// No tool calls → final answer
		if len(choice.Message.ToolCalls) == 0 {
			return &ChatCompletionResponse{
				Content:      choice.Message.Content,
				FinishReason: string(choice.FinishReason),
				Usage: Usage{
					PromptTokens:     resp.Usage.PromptTokens,
					CompletionTokens: resp.Usage.CompletionTokens,
					TotalTokens:      resp.Usage.TotalTokens,
				},
			}, nil
		}

		// Append the assistant message (with tool calls)
		messages = append(messages, openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   choice.Message.Content,
			ToolCalls: choice.Message.ToolCalls,
		})

		// Convert to our ToolCall type and execute
		calls := make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			calls[i] = ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments}
		}

		results, err := handler(ctx, calls)
		if err != nil {
			return nil, fmt.Errorf("tool execution failed (step %d): %w", step+1, err)
		}

		// Append tool results
		for _, r := range results {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    r.Content,
				ToolCallID: r.CallID,
			})
		}
	}

	return nil, fmt.Errorf("max tool calling steps (%d) exceeded", maxSteps)
}

// StreamChatCompletionWithToolLoop runs the tool-calling loop synchronously, then streams the
// final LLM response via callback. During tool execution, status messages are also sent to
// callback so the user can see what tools are being invoked.
//
// If the model does not support tool calling (e.g. deepseek-reasoner), the first API call
// will either return an error or return a regular response with no tool calls; in both cases
// the method falls back to a plain streaming call so the user always gets a response.
func (c *OpenAIClient) StreamChatCompletionWithToolLoop(
	ctx context.Context,
	req ChatCompletionRequest,
	tools []ToolDefinition,
	handler ToolCallHandler,
	maxSteps int,
	callback func(string) error,
) error {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	openaiTools := toOpenAITools(tools)
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = toOpenAIMessage(msg)
	}

	toolsInvoked := false

	for step := 0; step < maxSteps; step++ {
		apiReq := openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
		}
		if len(openaiTools) > 0 {
			apiReq.Tools = openaiTools
		}

		resp, err := c.client.CreateChatCompletion(ctx, apiReq)
		if err != nil {
			if step == 0 && !toolsInvoked {
				// Model likely doesn't support tools → fall back to regular streaming
				stream, sErr := c.CreateChatCompletionStream(ctx, req)
				if sErr != nil {
					return sErr
				}
				return c.StreamResponse(stream, callback)
			}
			return fmt.Errorf("chat completion failed (step %d): %w", step+1, err)
		}
		if len(resp.Choices) == 0 {
			return fmt.Errorf("no choices returned (step %d)", step+1)
		}

		choice := resp.Choices[0]

		// No tool calls → final answer
		if len(choice.Message.ToolCalls) == 0 {
			if toolsInvoked {
				_ = callback(ThoughtChunk("工具数据已准备完成，正在生成最终回答"))
				// Tool loop is done and we already have the final answer from the
				// non-streaming call above. Deliver it directly — no need to make
				// an extra streaming request (which could fail and leave the user
				// with a blank response).
				return callback(choice.Message.Content)
			}
			// No tools were invoked at all → stream the response normally
			_ = callback(ThoughtChunk("模型已接收请求，正在生成回答"))
			stream, sErr := c.CreateChatCompletionStream(ctx, req)
			if sErr != nil {
				return sErr
			}
			return c.StreamResponse(stream, callback)
		}

		toolsInvoked = true
		for _, tc := range choice.Message.ToolCalls {
			_ = callback(ThoughtChunk(fmt.Sprintf("正在调用工具：%s", tc.Function.Name)))
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   choice.Message.Content,
			ToolCalls: choice.Message.ToolCalls,
		})

		calls := make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			calls[i] = ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments}
		}

		results, err := handler(ctx, calls)
		if err != nil {
			return fmt.Errorf("tool execution failed (step %d): %w", step+1, err)
		}

		for i, r := range results {
			toolName := "unknown"
			if i < len(calls) {
				toolName = calls[i].Name
			}
			_ = callback(ThoughtChunk(fmt.Sprintf("工具执行完成：%s", toolName)))
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    r.Content,
				ToolCallID: r.CallID,
			})
		}
	}

	return fmt.Errorf("max tool calling steps (%d) exceeded", maxSteps)
}
