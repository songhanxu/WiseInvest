package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type searchFunc func(context.Context, string, int) ([]Result, error)

func (f searchFunc) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	return f(ctx, query, maxResults)
}

func TestDeepSeekSearchRequestAndResults(t *testing.T) {
	const key = "test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tools/search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request headers")
		}
		var body struct {
			Query       string `json:"query"`
			Count       int    `json:"count"`
			NeedContent bool   `json:"need_content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len([]rune(body.Query)) != 128 || body.Count != 50 || body.NeedContent {
			t.Errorf("unexpected search request: %+v", body)
		}
		_, _ = w.Write([]byte(`{"results":[{"title":"财报","url":"https://example.com/filing","snippet":"营收增长","summary":"summary","date":"2026-10-17T00:00:00Z"},{"title":"bad","url":"javascript:alert(1)","snippet":"ignore"}]}`))
	}))
	defer server.Close()
	s := &DeepSeekSearcher{apiKey: key, endpoint: server.URL + "/v1/tools/search", client: server.Client()}
	results, err := s.Search(context.Background(), strings.Repeat("财", 140), 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].URL != "https://example.com/filing" || results[0].Snippet != "营收增长" || results[0].Date != "2026-10-17T00:00:00Z" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestDeepSeekSearchStatusIsErrorWithoutCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"sensitive upstream detail"}}`))
	}))
	defer server.Close()
	s := &DeepSeekSearcher{apiKey: "test-secret", endpoint: server.URL, client: server.Client()}
	_, err := s.Search(context.Background(), "AAPL earnings", 5)
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "sensitive upstream detail") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFallbackSearcher(t *testing.T) {
	for _, tc := range []struct {
		name             string
		primaryResults   []Result
		primaryErr       error
		fallbackExpected bool
	}{
		{name: "primary success", primaryResults: []Result{{Title: "deepseek"}}},
		{name: "primary failure", primaryErr: errors.New("upstream error"), fallbackExpected: true},
		{name: "primary empty", fallbackExpected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fallbackCalls := 0
			s := &FallbackSearcher{
				primary: searchFunc(func(context.Context, string, int) ([]Result, error) { return tc.primaryResults, tc.primaryErr }),
				fallback: searchFunc(func(context.Context, string, int) ([]Result, error) {
					fallbackCalls++
					return []Result{{Title: "legacy"}}, nil
				}),
			}
			results, err := s.Search(context.Background(), "query", 5)
			if err != nil || (fallbackCalls == 1) != tc.fallbackExpected || len(results) != 1 {
				t.Fatalf("results=%+v err=%v fallback calls=%d", results, err, fallbackCalls)
			}
			if tc.fallbackExpected && results[0].Title != "legacy" {
				t.Fatalf("expected fallback result: %+v", results)
			}
		})
	}
}

func TestSearchProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		baseURL   string
		modelKey  string
		legacyKey string
		wantType  string
	}{
		{"official DeepSeek with legacy backup", "https://api.deepseek.com/v1", "model-key", "legacy-key", "*search.FallbackSearcher"},
		{"official DeepSeek only", "https://api.deepseek.com", "model-key", "", "*search.DeepSeekSearcher"},
		{"third party host", "https://example.com/v1", "model-key", "legacy-key", "*search.SerperSearcher"},
		{"missing model key", "https://api.deepseek.com/v1", "", "legacy-key", "*search.SerperSearcher"},
		{"insecure endpoint", "http://api.deepseek.com/v1", "model-key", "legacy-key", "*search.SerperSearcher"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewWithDeepSeek("serper", tc.legacyKey, tc.baseURL, tc.modelKey)
			if typeName := fmt.Sprintf("%T", got); typeName != tc.wantType {
				t.Fatalf("provider = %s, want %s", typeName, tc.wantType)
			}
		})
	}
}
