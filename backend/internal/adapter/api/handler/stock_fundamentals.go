package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
)

// Nullable values distinguish missing data from zero EPS. Guidance is sourced
// from an actual company filing, never from analyst consensus or an LLM guess.
type MetricProvenance struct {
	Source    string `json:"source"`
	SourceURL string `json:"source_url,omitempty"`
	Currency  string `json:"currency,omitempty"`
	AsOf      string `json:"as_of"`
}

type FundamentalsResponse struct {
	Description         string                      `json:"description,omitempty"`
	Metrics             map[string]MetricProvenance `json:"metrics,omitempty"`
	EPS                 *float64                    `json:"eps"`
	EPSBasis            string                      `json:"eps_basis,omitempty"`
	EPSPeriodStart      string                      `json:"eps_period_start,omitempty"`
	EPSPeriodEnd        string                      `json:"eps_period_end,omitempty"`
	EPSFiledDate        string                      `json:"eps_filed_date,omitempty"`
	SECCIK              string                      `json:"sec_cik,omitempty"`
	FinancialReports    []SECFinancialReport        `json:"financial_reports,omitempty"`
	PE                  *float64                    `json:"pe"`
	ForwardPE           *float64                    `json:"forward_pe"`
	ForwardEPS          *float64                    `json:"forward_eps"`
	ForwardPeriod       string                      `json:"forward_period,omitempty"`
	ForwardBasis        string                      `json:"forward_basis,omitempty"`
	GuidanceEPS         *float64                    `json:"guidance_eps"`
	GuidanceEPSLow      *float64                    `json:"guidance_eps_low"`
	GuidanceEPSHigh     *float64                    `json:"guidance_eps_high"`
	GuidancePeriod      string                      `json:"guidance_period,omitempty"`
	GuidanceBasis       string                      `json:"guidance_basis,omitempty"`
	GuidanceReportDate  string                      `json:"guidance_report_date,omitempty"`
	GuidanceSourceURL   string                      `json:"guidance_source_url,omitempty"`
	NextEarningsDate    string                      `json:"next_earnings_date,omitempty"`
	NextEarningsDateEnd string                      `json:"next_earnings_date_end,omitempty"`
	EarningsDateStatus  string                      `json:"earnings_date_status,omitempty"`
	Currency            string                      `json:"currency,omitempty"`
	Source              string                      `json:"source,omitempty"`
	AsOf                string                      `json:"as_of,omitempty"`
	Status              string                      `json:"status"`
	Message             string                      `json:"message,omitempty"`
}

type fundamentalsCache struct {
	Data FundamentalsResponse
	Time time.Time
}
type FundamentalsService struct {
	File                          string
	SEC                           *secFundamentalsClient
	Yahoo                         *yahooFundamentalsClient
	FinnhubAPIKey, FinnhubBaseURL string
	HTTPClient                    *http.Client
	mu                            sync.Mutex
	cache                         map[string]fundamentalsCache
	sem                           chan struct{}
	pending                       map[string]chan struct{}
}

func NewFundamentalsService(file string) *FundamentalsService {
	s := &FundamentalsService{File: file, HTTPClient: &http.Client{Timeout: 5 * time.Second}, cache: make(map[string]fundamentalsCache), sem: make(chan struct{}, 4), pending: make(map[string]chan struct{}), FinnhubAPIKey: os.Getenv("FINNHUB_API_KEY"), FinnhubBaseURL: "https://finnhub.io/api/v1"}
	if !strings.EqualFold(os.Getenv("SEC_FUNDAMENTALS_ENABLED"), "false") {
		s.SEC = newSECFundamentalsClient(os.Getenv("SEC_USER_AGENT"))
	}
	if !strings.EqualFold(os.Getenv("YAHOO_FUNDAMENTALS_ENABLED"), "false") {
		s.Yahoo = newYahooFundamentalsClient()
	}
	return s
}

func (s *FundamentalsService) loadVerified(market, code string) *FundamentalsResponse {
	if s.File == "" {
		return nil
	}
	data, err := os.ReadFile(s.File)
	if err != nil {
		return nil
	}
	var rows map[string]FundamentalsResponse
	if json.Unmarshal(data, &rows) != nil {
		return nil
	}
	row, ok := rows[market+":"+code]
	if !ok {
		return nil
	}
	if row.Source == "" || row.AsOf == "" {
		return nil
	}
	// Do not silently accept untraceable or old management guidance.
	reportDate, dateErr := time.Parse("2006-01-02", row.GuidanceReportDate)
	sourceURL, sourceErr := url.Parse(row.GuidanceSourceURL)
	if sourceErr != nil || sourceURL.Scheme != "https" || sourceURL.Host == "" || dateErr != nil || reportDate.After(time.Now()) || time.Since(reportDate) > 180*24*time.Hour || row.GuidancePeriod == "" || row.GuidanceBasis == "" {
		row.GuidanceEPS = nil
		row.GuidanceEPSLow = nil
		row.GuidanceEPSHigh = nil
		row.GuidancePeriod, row.GuidanceBasis, row.GuidanceReportDate, row.GuidanceSourceURL = "", "", "", ""
	}
	row.Status = "available"
	return &row
}

func (s *FundamentalsService) Fetch(ctx context.Context, market, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable"}
	if market == "crypto" {
		row.Status = "not_applicable"
		row.Message = "加密货币不适用 EPS、PE 和公司财报"
		return row
	}
	code = bitget.NormalizeCode(market, code)
	if market == "us_stock" {
		funds := map[string]bool{"SPY": true, "QQQ": true, "DIA": true, "IWM": true, "SMH": true, "GLD": true, "SLV": true, "VOO": true, "VTI": true, "SOXX": true, "TQQQ": true, "SQQQ": true, "ARKK": true, "TLT": true}
		if funds[code] {
			row.Status = "not_applicable"
			row.Message = "ETF / 基金不适用公司 EPS 与财报指引"
			return row
		}
	}
	verified := s.loadVerified(market, code)
	row = s.cachedFetch(ctx, market, code)
	if row.Status == "not_applicable" {
		return row
	}
	// Cached responses are shared by concurrent requests. Copy before applying
	// verified-file overrides so neither the cache nor other callers are mutated.
	metrics := make(map[string]MetricProvenance, len(row.Metrics))
	for key, value := range row.Metrics {
		metrics[key] = value
	}
	row.Metrics = metrics

	if verified != nil {
		if row.Description == "" {
			row.Description = verified.Description
		}
		if verified.EPS != nil && verified.Currency != "" && (row.Currency == "" || row.Currency == verified.Currency) {
			row.EPS = verified.EPS
			row.EPSBasis = verified.EPSBasis
			row.EPSPeriodStart, row.EPSPeriodEnd, row.EPSFiledDate = verified.EPSPeriodStart, verified.EPSPeriodEnd, verified.EPSFiledDate
			row.Metrics["eps"] = MetricProvenance{Source: verified.Source, Currency: verified.Currency, AsOf: verified.AsOf}
		}
		if verified.PE != nil {
			row.PE = verified.PE
			row.Metrics["pe"] = MetricProvenance{Source: verified.Source, AsOf: verified.AsOf}
		}
		if verified.ForwardPE != nil {
			row.ForwardPE = verified.ForwardPE
			row.ForwardEPS = verified.ForwardEPS
			row.Metrics["forward_pe"] = MetricProvenance{Source: verified.Source, AsOf: verified.AsOf}
			row.ForwardPeriod = verified.ForwardPeriod
			row.ForwardBasis = verified.ForwardBasis
			delete(row.Metrics, "forward_eps")
			if verified.ForwardEPS != nil {
				row.Metrics["forward_eps"] = MetricProvenance{Source: verified.Source, Currency: verified.Currency, AsOf: verified.AsOf}
			}
		}
		row.Metrics["guidance"] = MetricProvenance{Source: verified.Source, Currency: verified.Currency, AsOf: verified.GuidanceReportDate}
		row.GuidanceEPS = verified.GuidanceEPS
		row.GuidanceEPSLow = verified.GuidanceEPSLow
		row.GuidanceEPSHigh = verified.GuidanceEPSHigh
		row.GuidancePeriod = verified.GuidancePeriod
		row.GuidanceBasis = verified.GuidanceBasis
		row.GuidanceReportDate = verified.GuidanceReportDate
		row.GuidanceSourceURL = verified.GuidanceSourceURL
		if verified.NextEarningsDate != "" && verified.NextEarningsDate >= time.Now().UTC().Format("2006-01-02") {
			row.NextEarningsDate = verified.NextEarningsDate
			row.NextEarningsDateEnd = ""
			row.Metrics["next_earnings_date"] = MetricProvenance{Source: verified.Source, AsOf: verified.AsOf}
			row.EarningsDateStatus = verified.EarningsDateStatus
		}
		if row.Source != "" {
			row.Source += " + "
		}
		row.Source += verified.Source
		if row.AsOf == "" {
			row.AsOf = verified.AsOf
		}
		if row.EPS != nil || row.PE != nil || row.ForwardPE != nil || row.GuidanceEPS != nil || row.GuidanceEPSLow != nil || row.NextEarningsDate != "" {
			row.Status = "available"
			row.Message = ""
		}
		if row.Currency == "" {
			row.Currency = verified.Currency
		}
	}
	return row
}

func (h *StockHandler) GetFundamentals(c *gin.Context) {
	market, code := c.DefaultQuery("market", "us_stock"), c.Query("code")
	if !validStockMarket(market) || code == "" || len(code) > 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid market and code are required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 13*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, h.fundamentals.Fetch(ctx, market, code))
}

func (s *FundamentalsService) cachedFetch(ctx context.Context, market, code string) FundamentalsResponse {
	key := market + ":" + code
	for {
		s.mu.Lock()
		if entry, ok := s.cache[key]; ok && time.Since(entry.Time) < 6*time.Hour {
			s.mu.Unlock()
			return entry.Data
		}
		if done := s.pending[key]; done != nil {
			s.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return FundamentalsResponse{Status: "unavailable", Message: "财报查询超时，请稍后重试"}
			}
		}
		if s.pending == nil {
			s.pending = make(map[string]chan struct{})
		}
		done := make(chan struct{})
		s.pending[key] = done
		s.mu.Unlock()
		go func() {
			taskCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			row := FundamentalsResponse{Status: "unavailable", Message: "财报查询繁忙，请稍后重试"}
			select {
			case s.sem <- struct{}{}:
				row = s.fetchSources(taskCtx, market, code)
				<-s.sem
			case <-taskCtx.Done():
			}
			stamp := time.Now()
			// Partial results must recover promptly when an upstream subscription,
			// session or network issue is resolved, rather than staying empty for 6h.
			if row.Status == "unavailable" || (row.Status == "available" && (row.EPS == nil || row.ForwardPE == nil || row.NextEarningsDate == "")) {
				stamp = stamp.Add(-6*time.Hour + time.Minute)
			}
			s.mu.Lock()
			for len(s.cache) >= 512 {
				oldestKey := ""
				var oldest time.Time
				for k, v := range s.cache {
					if oldestKey == "" || v.Time.Before(oldest) {
						oldestKey, oldest = k, v.Time
					}
				}
				delete(s.cache, oldestKey)
			}
			s.cache[key] = fundamentalsCache{Data: row, Time: stamp}
			delete(s.pending, key)
			close(done)
			s.mu.Unlock()
		}()
		select {
		case <-done:
		case <-ctx.Done():
			return FundamentalsResponse{Status: "unavailable", Message: "财报查询超时，请稍后重试"}
		}
	}
}
