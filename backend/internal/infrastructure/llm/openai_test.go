package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestThinkingControlIsScopedToOptedInDeepSeekCompletions(t *testing.T) {
	var modes []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Thinking  *struct {
				Type string `json:"type"`
			} `json:"thinking"`
			Messages []ChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/chat/completions" || body.Model != "deepseek-v4-flash" || body.MaxTokens != 8192 || len(body.Messages) != 1 || body.Messages[0].Content != "资料" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Errorf("extension changed the original request: %+v", body)
		}
		mode := ""
		if body.Thinking != nil {
			mode = body.Thinking.Type
		}
		mu.Lock()
		modes = append(modes, mode)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"完整正文","reasoning_content":"private analysis"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`)
	}))
	defer server.Close()
	client := NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "deepseek-v4-flash"})
	// Route an official-provider request to the local fixture without real API access.
	client.deepSeekEndpoint = true
	for _, mode := range []string{"disabled", "", "enabled", ""} {
		answer, err := client.CreateChatCompletion(context.Background(), ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "资料"}}, MaxTokens: 8192, ThinkingMode: mode})
		if err != nil || answer.Content != "完整正文" || answer.Usage.CompletionTokens != 20 || answer.FinishReason != "stop" {
			t.Fatalf("wrong completion: %+v %v", answer, err)
		}
	}
	client.deepSeekEndpoint = false
	_, err := client.CreateChatCompletion(context.Background(), ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "资料"}}, MaxTokens: 8192, ThinkingMode: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(modes, ",") != "disabled,,enabled,," {
		t.Fatalf("thinking extension leaked to ordinary or third-party requests: %q", modes)
	}
}

func TestThinkingControlOnlySupportsKnownOfficialModels(t *testing.T) {
	for _, test := range []struct {
		base, model string
		want        bool
	}{
		{"https://api.deepseek.com/v1", "deepseek-v4-flash", true},
		{"https://api.deepseek.com", "deepseek-v4-pro", true},
		{"https://api.deepseek.com", "deepseek-flash", true},
		{"https://api.deepseek.com", "deepseek-reasoner", false},
		{"https://api.openai.com/v1", "gpt-4-turbo-preview", false},
		{"https://third-party.example/v1", "deepseek-v4-flash", false},
	} {
		client := NewOpenAIClient(config.OpenAIConfig{BaseURL: test.base, Model: test.model})
		if client.SupportsThinkingControl() != test.want {
			t.Errorf("wrong thinking capability for %s %s", test.base, test.model)
		}
	}
}

func TestProOverrideSendsMaxReasoningOnlyOnOfficialDeepSeek(t *testing.T) {
	var bodies []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"解读"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	client := NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "deepseek-flash", ModelPro: "deepseek-v4-pro"})
	client.deepSeekEndpoint = true
	request := ChatCompletionRequest{Model: client.ProModel(), Messages: []ChatMessage{{Role: "user", Content: "财报"}}, ThinkingMode: "enabled", ReasoningEffort: "max", MaxTokens: 32768}
	if _, err := client.CreateChatCompletion(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateChatCompletion(context.Background(), ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "普通聊天"}}}); err != nil {
		t.Fatal(err)
	}
	client.deepSeekEndpoint = false
	if _, err := client.CreateChatCompletion(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 3 || string(bodies[0]["model"]) != `"deepseek-v4-pro"` || string(bodies[0]["reasoning_effort"]) != `"max"` || string(bodies[0]["thinking"]) != `{"type":"enabled"}` || string(bodies[0]["max_tokens"]) != "32768" {
		t.Fatalf("Pro request options missing: %+v", bodies)
	}
	if string(bodies[1]["model"]) != `"deepseek-flash"` || bodies[1]["reasoning_effort"] != nil || bodies[1]["thinking"] != nil || bodies[2]["reasoning_effort"] != nil || bodies[2]["thinking"] != nil {
		t.Fatalf("Pro options leaked to other requests: %+v", bodies)
	}
}

func TestChatSelectionControlsStreamingModelAndReasoning(t *testing.T) {
	var sent map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"回答\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "deepseek-flash", ModelPro: "deepseek-v4-pro"})
	client.deepSeekEndpoint = true
	for _, bad := range []ChatSelection{{Model: "unapproved-model", Effort: "max"}, {Model: "deepseek-v4-pro", Effort: "ultra"}} {
		if _, err := client.ValidateChatSelection(bad.Model, bad.Effort); err == nil {
			t.Fatalf("accepted invalid chat selection: %+v", bad)
		}
	}
	selection, err := client.ValidateChatSelection("deepseek-v4-pro", "max")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithChatSelection(context.Background(), selection)
	if client.SupportsToolCalling(ctx) {
		t.Fatal("thinking mode entered a tool loop without reasoning context support")
	}
	stream, err := client.CreateChatCompletionStream(ctx, ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "问题"}}, MaxTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	var answer strings.Builder
	if err := client.StreamResponse(stream, func(chunk string) error { answer.WriteString(chunk); return nil }); err != nil || answer.String() != "回答" {
		t.Fatalf("streaming answer failed: %q %v", answer.String(), err)
	}
	if string(sent["model"]) != `"deepseek-v4-pro"` || string(sent["thinking"]) != `{"type":"enabled"}` || string(sent["reasoning_effort"]) != `"max"` || string(sent["max_tokens"]) != "32768" {
		t.Fatalf("chat selection not sent to model: %+v", sent)
	}
}

func TestChatSelectionWithoutThinkingKeepsToolCalls(t *testing.T) {
	var sent map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"完成"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	client := NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "deepseek-flash", ModelPro: "deepseek-v4-pro"})
	client.deepSeekEndpoint = true
	selection, err := client.ValidateChatSelection("deepseek-flash", "none")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithChatSelection(context.Background(), selection)
	if !client.SupportsToolCalling(ctx) {
		t.Fatal("non-thinking selection disabled tool calls")
	}
	if _, err := client.CreateChatCompletionWithToolLoop(ctx, ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: "问题"}}, MaxTokens: 4000}, nil, nil, 1); err != nil {
		t.Fatal(err)
	}
	if string(sent["model"]) != `"deepseek-flash"` || string(sent["thinking"]) != `{"type":"disabled"}` || sent["reasoning_effort"] != nil {
		t.Fatalf("non-thinking options were not applied: %+v", sent)
	}
}
