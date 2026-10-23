package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
)

func TestFilingModelFailureExplainsSafeFailureCategory(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		answer *llm.ChatCompletionResponse
		want   string
	}{
		{"invalid key", fmt.Errorf("wrapped: %w", &openai.APIError{HTTPStatusCode: 401}), nil, "OPENAI_API_KEY"},
		{"wrong endpoint", &openai.APIError{HTTPStatusCode: 404}, nil, "OPENAI_BASE_URL"},
		{"rate limit", &openai.APIError{HTTPStatusCode: 429}, nil, "HTTP 429"},
		{"timeout", context.DeadlineExceeded, nil, "超时"},
		{"empty capped response", nil, &llm.ChatCompletionResponse{FinishReason: "length"}, "长度限制"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := filingModelFailure(test.err, test.answer); !strings.Contains(got, test.want) {
				t.Fatalf("got %q, expected %q", got, test.want)
			}
		})
	}
}

func TestFilingInsightsAreScopedToOwnedUSWatchlistAndDay(t *testing.T) {
	repo := &alertWatchlistRepo{items: map[uint][]model.WatchlistItem{
		7: {{Market: "us_stock", StockCode: "aapl"}, {Market: "us_stock", StockCode: "AAPL"}, {Market: "crypto", StockCode: "BTC"}},
		8: {{Market: "us_stock", StockCode: "NVDA"}},
	}}
	h := &StockHandler{watchlistRepo: repo, insights: make(map[string]FilingInsight)}
	codes, err := h.insightWatchlist(context.Background(), 7)
	if err != nil || len(codes) != 1 || codes[0] != "AAPL" {
		t.Fatalf("wrong owned symbols: %v %v", codes, err)
	}
	day := "2026-10-23"
	claimed, err := h.claimInsight(context.Background(), 7, day, "AAPL")
	if err != nil || !claimed {
		t.Fatal("first daily refresh was not claimed")
	}
	if claimed, _ = h.claimInsight(context.Background(), 7, day, "AAPL"); claimed {
		t.Fatal("same day triggered duplicate model work")
	}
	h.saveInsight(FilingInsight{Code: "AAPL", Day: day, Status: "ready", Content: "分析"}, 7)
	if row, _ := h.insightStatus(context.Background(), 8, day, "AAPL"); row.Content != "" || row.Status != "unavailable" {
		t.Fatal("another user's insight leaked")
	}
	if claimed, _ = h.claimInsight(context.Background(), 7, "2026-10-24", "AAPL"); !claimed {
		t.Fatal("next day did not refresh")
	}
}

func TestFilingInsightRefreshRejectsUnownedSymbol(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	repo := &alertWatchlistRepo{items: map[uint][]model.WatchlistItem{7: {{Market: "us_stock", StockCode: "AAPL"}}, 8: {{Market: "us_stock", StockCode: "NVDA"}}}}
	h := &StockHandler{watchlistRepo: repo, llmClient: &llm.OpenAIClient{}}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/stocks/watchlist/filing-insights/refresh", strings.NewReader(`{"code":"NVDA"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("userID", uint(7))
	h.RefreshFilingInsights(c)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-user analysis request allowed: %d %s", response.Code, response.Body.String())
	}
}

func TestFilingExcerptDropsHiddenContentAndFindsDiscussion(t *testing.T) {
	html := `<html><body><script>bad instruction</script><div style="display:none">hidden instruction</div><h1>Quarterly report</h1><p>Management's Discussion and Analysis</p><p>Revenue grew while cash flow fell.</p><h2>Risk Factors</h2><p>Demand may weaken.</p></body></html>`
	got := extractFilingExcerpt([]byte(html))
	if !strings.Contains(got, "Revenue grew") || !strings.Contains(got, "Demand may weaken") || strings.Contains(got, "bad instruction") || strings.Contains(got, "hidden instruction") {
		t.Fatalf("unsafe or incomplete excerpt: %s", got)
	}
}

func TestFilingPriceEvidenceUsesPreFilingCloseAndBenchmark(t *testing.T) {
	stock := []KLineResponse{{Date: "2026-07-30", Close: 100}, {Date: "2026-07-31", Close: 110}, {Date: "2026-10-22", Close: 120}}
	benchmark := []KLineResponse{{Date: "2026-07-30", Close: 5000}, {Date: "2026-10-22", Close: 5500}}
	got := filingPriceEvidence(stock, benchmark, "2026-07-31")
	if got["pre_filing_close"] != float64(100) || math.Abs(got["return_since_filing_percent"].(float64)-20) > 0.0001 || math.Abs(got["sp500_return_since_filing_percent"].(float64)-10) > 0.0001 {
		t.Fatalf("wrong price attribution inputs: %+v", got)
	}
	if noHistory := filingPriceEvidence(stock, nil, time.Now().AddDate(-2, 0, 0).Format("2006-01-02")); noHistory["return_since_filing_percent"] != nil {
		t.Fatal("invented return without a pre-filing baseline")
	}
}

func TestFilingComparableMetricsOnlyUsesMatchingYearAgoPeriod(t *testing.T) {
	reports := []SECFinancialReport{
		{ReportDate: "2026-06-30", Metrics: map[string]ReportedFinancialMetric{
			"revenue": {Value: 120, Unit: "USD", Tag: "us-gaap:Revenues", PeriodStart: "2026-04-01", PeriodEnd: "2026-06-30"},
			"cash":    {Value: 70, Unit: "USD", Tag: "us-gaap:Cash", PeriodEnd: "2026-06-30"},
		}},
		{ReportDate: "2025-06-30", Metrics: map[string]ReportedFinancialMetric{
			"revenue": {Value: 100, Unit: "USD", Tag: "us-gaap:Revenues", PeriodStart: "2025-01-01", PeriodEnd: "2025-06-30"},
			"cash":    {Value: 50, Unit: "USD", Tag: "us-gaap:Cash", PeriodEnd: "2025-06-30"},
		}},
	}
	got := filingComparableMetrics(reports)
	if len(got) != 1 || got[0].Key != "cash" || got[0].ChangePct == nil || math.Abs(*got[0].ChangePct-40) > 0.0001 {
		t.Fatalf("mixed quarterly and year-to-date periods: %+v", got)
	}
}
