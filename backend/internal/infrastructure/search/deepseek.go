package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const deepSeekSearchURL = "https://api.deepseek.com/v1/tools/search"

// DeepSeekSearcher calls DeepSeek's Search API, independently of chat completions.
type DeepSeekSearcher struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func (s *DeepSeekSearcher) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("deepseek search query is empty")
	}
	// The API accepts at most 128 Unicode characters. Queries from chat can be longer.
	if runes := []rune(query); len(runes) > 128 {
		query = string(runes[:128])
	}
	if maxResults < 1 {
		maxResults = 1
	} else if maxResults > 50 {
		maxResults = 50
	}
	body, err := json.Marshal(struct {
		Query       string `json:"query"`
		Count       int    `json:"count"`
		NeedContent bool   `json:"need_content"`
	}{Query: query, Count: maxResults, NeedContent: false})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek search request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek search returned HTTP %d", resp.StatusCode)
	}
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
			Summary string `json:"summary"`
			Date    string `json:"date"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("deepseek search response invalid: %w", err)
	}
	results := make([]Result, 0, len(data.Results))
	for _, item := range data.Results {
		address, err := url.Parse(item.URL)
		if err != nil || (address.Scheme != "https" && address.Scheme != "http") || address.Host == "" {
			continue
		}
		snippet := strings.TrimSpace(item.Snippet)
		if snippet == "" {
			snippet = strings.TrimSpace(item.Summary)
		}
		if chars := []rune(snippet); len(chars) > 800 {
			snippet = string(chars[:800])
		}
		results = append(results, Result{Title: item.Title, URL: item.URL, Snippet: snippet, Date: item.Date})
	}
	return results, nil
}

// FallbackSearcher keeps the existing search provider available if DeepSeek
// search is unavailable or finds nothing.
type FallbackSearcher struct {
	primary  Searcher
	fallback Searcher
}

func (s *FallbackSearcher) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	results, primaryErr := s.primary.Search(ctx, query, maxResults)
	if primaryErr == nil && len(results) > 0 {
		return results, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	results, fallbackErr := s.fallback.Search(ctx, query, maxResults)
	if fallbackErr != nil {
		return nil, errors.Join(primaryErr, fallbackErr)
	}
	return results, nil
}
