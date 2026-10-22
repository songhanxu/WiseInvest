// Package search provides web search capabilities for agents.
// When configured with a search API key, agents can retrieve real-time
// information (stock news, company announcements, market data, etc.)
//
// Supported providers (set SEARCH_PROVIDER + SEARCH_API_KEY in .env):
//   - "serper"  → Serper.dev  (https://serper.dev, free tier: 2500 queries/month)
//   - "bing"    → Bing Search API
//   - "brave"   → Brave Search API (https://brave.com/search/api, free tier: 2000/month)
//
// If SEARCH_API_KEY is empty, NoopSearcher is used (returns empty results).
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Result is a single web search result.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Date    string `json:"date,omitempty"` // Publication date from search engine (e.g. "2026-03-23", "3 hours ago")
}

// Searcher is the interface for performing web searches.
type Searcher interface {
	Search(ctx context.Context, query string, maxResults int) ([]Result, error)
}

// New returns the appropriate Searcher based on configuration.
// Falls back to NoopSearcher when no API key is provided.
func New(provider, apiKey string) Searcher {
	if apiKey == "" {
		return &NoopSearcher{}
	}
	switch provider {
	case "serper":
		return &SerperSearcher{apiKey: apiKey}
	case "brave":
		return &BraveSearcher{apiKey: apiKey}
	default:
		return &NoopSearcher{}
	}
}

// --- NoopSearcher (default, no API key required) ---

type NoopSearcher struct{}

func (n *NoopSearcher) Search(_ context.Context, _ string, _ int) ([]Result, error) {
	return nil, nil // Web search not configured; agents will use training data
}

// --- SerperSearcher (https://serper.dev) ---

type SerperSearcher struct {
	apiKey string
}

func (s *SerperSearcher) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	// Try past week first; if too few results, fall back to past month
	results, err := s.searchWithTbs(ctx, query, maxResults, "qdr:w")
	if err != nil {
		return nil, err
	}
	if len(results) < 3 {
		expanded, err2 := s.searchWithTbs(ctx, query, maxResults, "qdr:m")
		if err2 == nil && len(expanded) > len(results) {
			return expanded, nil
		}
	}
	return results, nil
}

func (s *SerperSearcher) searchWithTbs(ctx context.Context, query string, maxResults int, tbs string) ([]Result, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"q": query, "num": maxResults, "hl": "zh-cn", "gl": "cn",
		"tbs": tbs,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://google.serper.dev/search", bytes.NewReader(body))
	req.Header.Set("X-API-KEY", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serper search failed: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
			Date    string `json:"date"`
		} `json:"organic"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("serper decode failed: %w", err)
	}

	results := make([]Result, 0, len(data.Organic))
	for _, item := range data.Organic {
		results = append(results, Result{Title: item.Title, URL: item.Link, Snippet: item.Snippet, Date: item.Date})
	}
	return results, nil
}

// --- BraveSearcher (https://brave.com/search/api) ---

type BraveSearcher struct {
	apiKey string
}

func (b *BraveSearcher) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	// Try past week first; if too few results, fall back to past month
	results, err := b.searchWithFreshness(ctx, query, maxResults, "pw")
	if err != nil {
		return nil, err
	}
	if len(results) < 3 {
		expanded, err2 := b.searchWithFreshness(ctx, query, maxResults, "pm")
		if err2 == nil && len(expanded) > len(results) {
			return expanded, nil
		}
	}
	return results, nil
}

func (b *BraveSearcher) searchWithFreshness(ctx context.Context, query string, maxResults int, freshness string) ([]Result, error) {
	endpoint := fmt.Sprintf(
		"https://api.search.brave.com/res/v1/web/search?q=%s&count=%d&freshness=%s",
		url.QueryEscape(query), maxResults, freshness,
	)
	req, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search failed: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				PageAge     string `json:"page_age"`
				Age         string `json:"age"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("brave decode failed: %w", err)
	}

	results := make([]Result, 0, len(data.Web.Results))
	for _, item := range data.Web.Results {
		date := item.PageAge
		if date == "" {
			date = item.Age
		}
		results = append(results, Result{Title: item.Title, URL: item.URL, Snippet: item.Description, Date: date})
	}
	return results, nil
}
