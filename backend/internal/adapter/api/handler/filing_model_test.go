package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
)

func TestFilingLengthFailureRetriesOnceFromOriginalEvidence(t *testing.T) {
	for _, partial := range []string{"", "不完整观点，不能当作完整结论"} {
		t.Run(fmt.Sprintf("partial=%t", partial != ""), func(t *testing.T) {
			var calls atomic.Int32
			var originalEvidence string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					MaxTokens int               `json:"max_tokens"`
					Messages  []llm.ChatMessage `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				call := calls.Add(1)
				if len(req.Messages) != 2 {
					t.Errorf("incomplete reply was appended to prompt: %+v", req.Messages)
					w.WriteHeader(400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if call == 1 {
					originalEvidence = req.Messages[1].Content
					if req.MaxTokens != 8192 {
						t.Errorf("first budget = %d", req.MaxTokens)
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": partial}, "finish_reason": "length"}}})
					return
				}
				if call != 2 || req.MaxTokens != 16384 || req.Messages[1].Content != originalEvidence || !strings.Contains(req.Messages[0].Content, "完整收尾") {
					t.Errorf("wrong bounded retry: %+v", req)
				}
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"完整解读与风险"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			h := &StockHandler{llmClient: llm.NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "fixture-model"})}
			answer, err := h.requestFilingInsight(context.Background(), "TEST", []byte(`{"code":"TEST","reported_eps":1.2}`))
			if err != nil || !filingAnswerComplete(answer) || answer.Content != "完整解读与风险" || calls.Load() != 2 {
				t.Fatalf("length failure did not recover: %+v %v calls=%d", answer, err, calls.Load())
			}
		})
	}
}

func TestFilingRetryBudgetAndNonRetryableResults(t *testing.T) {
	for _, test := range []struct {
		name, reason, content string
		status, calls         int
		ready                 bool
	}{
		{"complete", "stop", "完整解读", 200, 1, true},
		{"empty", "stop", "", 200, 1, false},
		{"still truncated", "length", "半段内容", 200, 2, false},
		{"filtered", "content_filter", "部分内容", 200, 1, false},
		{"unfinished", "", "部分内容", 200, 1, false},
		{"invalid key", "", "", 401, 1, false},
		{"rate limit", "", "", 429, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				call := calls.Add(1)
				wantBudget := 2048
				if call == 2 {
					wantBudget = 4096
				}
				if req.MaxTokens != wantBudget {
					t.Errorf("configured budget ignored: got=%d want=%d", req.MaxTokens, wantBudget)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				if test.status != 200 {
					fmt.Fprint(w, `{"error":{"message":"fixture failure","type":"test"}}`)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": test.content}, "finish_reason": test.reason}}})
			}))
			defer server.Close()
			settings := config.DefaultFilingInsightConfig()
			settings.MaxTokens, settings.RetryMaxTokens = 2048, 4096
			h := &StockHandler{filingConfig: settings, llmClient: llm.NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: server.URL, Model: "fixture-model"})}
			answer, err := h.requestFilingInsight(context.Background(), "TEST", []byte(`{}`))
			if (err == nil && filingAnswerComplete(answer)) != test.ready || calls.Load() != int32(test.calls) {
				t.Fatalf("wrong result or retry count: %+v %v calls=%d", answer, err, calls.Load())
			}
		})
	}
}

func TestFilingCanceledJobDoesNotCallModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := &StockHandler{llmClient: llm.NewOpenAIClient(config.OpenAIConfig{BaseURL: "http://127.0.0.1:1", Model: "fixture-model"})}
	if _, err := h.requestFilingInsight(ctx, "TEST", nil); err != context.Canceled {
		t.Fatalf("cancel not preserved: %v", err)
	}
}

type filingTestTransport func(*http.Request) (*http.Response, error)

func (f filingTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFilingGenerationNeverCachesTruncatedAnalysisAsReady(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry recovers=%t", recover), func(t *testing.T) {
			facts, submissions := secExampleData(time.Now())
			sec := secFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "companyfacts"):
					json.NewEncoder(w).Encode(facts)
				case strings.Contains(r.URL.Path, "submissions"):
					json.NewEncoder(w).Encode(submissions)
				default:
					fmt.Fprint(w, `{"0":{"cik_str":123,"ticker":"TEST"}}`)
				}
			})
			// All data sources are local fixtures. Missing price history remains
			// missing; it must not prevent us from exercising model publication.
			sec.client.Transport = filingTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "www.sec.gov" {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("<p>Revenue and cash flow discussion.</p>")), Request: r}, nil
				}
				return http.DefaultTransport.RoundTrip(r)
			})
			var calls atomic.Int32
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				reason, content := "length", "半段解读"
				if recover && call == 2 {
					reason, content = "stop", "完整解读与风险"
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": reason}}})
			}))
			defer model.Close()
			h := &StockHandler{
				fundamentals: &FundamentalsService{SEC: sec}, insights: make(map[string]FilingInsight),
				insightCtx: context.Background(), insightSem: make(chan struct{}, 1), llmSem: make(chan struct{}, 1),
				llmClient: llm.NewOpenAIClient(config.OpenAIConfig{APIKey: "test-only", BaseURL: model.URL, Model: "fixture-model"}),
				httpClient: &http.Client{Transport: filingTestTransport(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: r}, nil
				})},
			}
			day := insightDay(time.Now())
			h.generateFilingInsight(7, day, "TEST")
			result, err := h.insightStatus(context.Background(), 7, day, "TEST")
			if err != nil || calls.Load() != 2 {
				t.Fatalf("fixture generation did not reach retry: %+v %v calls=%d", result, err, calls.Load())
			}
			if recover {
				if result.Status != "ready" || result.Content != "完整解读与风险" || result.GeneratedAt == "" || result.Accession == "" {
					t.Fatalf("complete retry was not published: %+v", result)
				}
			} else if result.Status != "failed" || result.Content != "" || result.GeneratedAt != "" || !strings.Contains(result.Message, "长度限制") {
				t.Fatalf("truncated analysis published as success: %+v", result)
			}
		})
	}
}
