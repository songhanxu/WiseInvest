package llm

import (
	"context"
	"fmt"
	"io"

	openai "github.com/sashabaranov/go-openai"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

// OpenAIClient wraps the OpenAI client
type OpenAIClient struct {
	client *openai.Client
	model  string
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

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string
	Content string
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

// CreateChatCompletion creates a chat completion
func (c *OpenAIClient) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
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
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
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
