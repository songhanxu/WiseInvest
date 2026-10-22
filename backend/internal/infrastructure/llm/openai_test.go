package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

func TestToolLoopDeliversCompletedAnswerWithoutSecondRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			http.Error(w, "stream unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"已有的完整观点"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	client := NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "test-model"})
	var answer strings.Builder
	err := client.StreamChatCompletionWithToolLoop(context.Background(), ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "研究视角"}}, MaxTokens: 100}, []ToolDefinition{{Name: "test_tool"}}, nil, 1, func(chunk string) error { answer.WriteString(chunk); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || answer.String() != "已有的完整观点" {
		t.Fatalf("requests=%d answer=%q", requests, answer.String())
	}
}

func TestToolLoopRejectsEmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	client := NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "test-model"})
	err := client.StreamChatCompletionWithToolLoop(context.Background(), ChatCompletionRequest{}, nil, nil, 1, func(string) error { t.Fatal("must not emit empty success"); return nil })
	if err == nil || !strings.Contains(err.Error(), "empty answer") {
		t.Fatalf("unexpected error: %v", err)
	}
}
