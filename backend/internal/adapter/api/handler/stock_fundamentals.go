package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
)

// Nullable values distinguish missing data from zero EPS. Guidance is sourced
// from an actual company filing, never from analyst consensus or an LLM guess.
type FundamentalsResponse struct {
	EPS                *float64 `json:"eps"`
	PE                 *float64 `json:"pe"`
	ForwardPE          *float64 `json:"forward_pe"`
	ForwardEPS         *float64 `json:"forward_eps"`
	ForwardPeriod      string   `json:"forward_period,omitempty"`
	GuidanceEPS        *float64 `json:"guidance_eps"`
	GuidanceEPSLow     *float64 `json:"guidance_eps_low"`
	GuidanceEPSHigh    *float64 `json:"guidance_eps_high"`
	GuidancePeriod     string   `json:"guidance_period,omitempty"`
	GuidanceBasis      string   `json:"guidance_basis,omitempty"`
	GuidanceReportDate string   `json:"guidance_report_date,omitempty"`
	GuidanceSourceURL  string   `json:"guidance_source_url,omitempty"`
	NextEarningsDate   string   `json:"next_earnings_date,omitempty"`
	EarningsDateStatus string   `json:"earnings_date_status,omitempty"`
	Currency           string   `json:"currency,omitempty"`
	Source             string   `json:"source,omitempty"`
	AsOf               string   `json:"as_of,omitempty"`
	Status             string   `json:"status"`
	Message            string   `json:"message,omitempty"`
}

type fundamentalsCache struct {
	Data FundamentalsResponse
	Time time.Time
}
type FundamentalsService struct {
	APIKey, File, BaseURL string
	HTTPClient            *http.Client
	mu                    sync.Mutex
	cache                 map[string]fundamentalsCache
	sem                   chan struct{}
}

func NewFundamentalsService(key, file string) *FundamentalsService {
	return &FundamentalsService{APIKey: key, File: file, BaseURL: "https://financialmodelingprep.com/stable", HTTPClient: &http.Client{Timeout: 5 * time.Second}, cache: make(map[string]fundamentalsCache), sem: make(chan struct{}, 4)}
}

func (s *FundamentalsService) get(ctx context.Context, path, symbol string, params url.Values, target interface{}) bool {
	if params == nil {
		params = url.Values{}
	}
	params.Set("symbol", symbol)
	params.Set("apikey", s.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/"+path+"?"+params.Encode(), nil)
	if err != nil {
		return false
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target) == nil
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
	if row.GuidanceSourceURL == "" || row.GuidanceReportDate == "" || row.GuidancePeriod == "" || row.GuidanceBasis == "" {
		row.GuidanceEPS = nil
		row.GuidanceEPSLow = nil
		row.GuidanceEPSHigh = nil
	}
	row.Status = "available"
	return &row
}

func (s *FundamentalsService) Fetch(ctx context.Context, market, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable", Message: "Bitget 不提供基本面；尚未配置财报数据源"}
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
	if s.APIKey != "" {
		key := market + ":" + code
		s.mu.Lock()
		cached, ok := s.cache[key]
		s.mu.Unlock()
		if ok && time.Since(cached.Time) < 6*time.Hour {
			row = cached.Data
		} else {
			select {
			case s.sem <- struct{}{}:
				defer func() { <-s.sem }()
			case <-ctx.Done():
				return row
			}
			row = s.fetchFMP(ctx, market, code)
			ttlNow := time.Now()
			if row.Status == "unavailable" {
				ttlNow = ttlNow.Add(-6*time.Hour + time.Minute)
			}
			s.mu.Lock()
			s.cache[key] = fundamentalsCache{row, ttlNow}
			s.mu.Unlock()
		}
	}
	if verified != nil {
		if verified.EPS != nil {
			row.EPS = verified.EPS
		}
		if verified.PE != nil {
			row.PE = verified.PE
		}
		if verified.ForwardPE != nil {
			row.ForwardPE = verified.ForwardPE
			row.ForwardPeriod = verified.ForwardPeriod
		}
		row.GuidanceEPS = verified.GuidanceEPS
		row.GuidanceEPSLow = verified.GuidanceEPSLow
		row.GuidanceEPSHigh = verified.GuidanceEPSHigh
		row.GuidancePeriod = verified.GuidancePeriod
		row.GuidanceBasis = verified.GuidanceBasis
		row.GuidanceReportDate = verified.GuidanceReportDate
		row.GuidanceSourceURL = verified.GuidanceSourceURL
		if verified.NextEarningsDate != "" && verified.NextEarningsDate >= time.Now().UTC().Format("2006-01-02") {
			row.NextEarningsDate = verified.NextEarningsDate
			row.EarningsDateStatus = verified.EarningsDateStatus
		}
		if row.Source != "" {
			row.Source += " + "
		}
		row.Source += verified.Source
		row.AsOf = verified.AsOf
		row.Status = "available"
		row.Message = ""
		if row.Currency == "" {
			row.Currency = verified.Currency
		}
	}
	return row
}

func (s *FundamentalsService) fetchFMP(ctx context.Context, market, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable", Message: "基本面数据暂不可用或当前数据订阅未覆盖该标的"}
	symbol := code
	if market == "hk_stock" {
		symbol = strings.TrimLeft(code, "0") + ".HK"
		for len(symbol) < 7 {
			symbol = "0" + symbol
		}
	}
	if market == "a_share" {
		symbol = strings.TrimPrefix(strings.TrimPrefix(code, "sh"), "sz")
		if strings.HasPrefix(code, "sh") || strings.HasPrefix(symbol, "6") {
			symbol += ".SS"
		} else {
			symbol += ".SZ"
		}
	}
	var income []struct {
		EPS      *float64 `json:"epsDiluted"`
		Currency string   `json:"reportedCurrency"`
	}
	var ratios []struct {
		PE *float64 `json:"priceToEarningsRatioTTM"`
	}
	var estimates []struct {
		Date string   `json:"date"`
		EPS  *float64 `json:"epsAvg"`
	}
	var earnings []struct {
		Date string `json:"date"`
	}
	var profiles []struct {
		Price    *float64 `json:"price"`
		Currency string   `json:"currency"`
		IsETF    bool     `json:"isEtf"`
		IsFund   bool     `json:"isFund"`
	}
	var wg sync.WaitGroup
	for _, job := range []func(){
		func() { s.get(ctx, "income-statement-ttm", symbol, nil, &income) },
		func() { s.get(ctx, "ratios-ttm", symbol, nil, &ratios) },
		func() {
			s.get(ctx, "analyst-estimates", symbol, url.Values{"period": {"annual"}, "limit": {"10"}}, &estimates)
		},
		func() { s.get(ctx, "earnings", symbol, url.Values{"limit": {"20"}}, &earnings) },
		func() { s.get(ctx, "profile", symbol, nil, &profiles) },
	} {
		wg.Add(1)
		go func(job func()) { defer wg.Done(); job() }(job)
	}
	wg.Wait()
	if len(profiles) > 0 && (profiles[0].IsETF || profiles[0].IsFund) {
		row.Status = "not_applicable"
		row.Message = "ETF / 基金不适用公司 EPS 与财报指引"
		return row
	}
	if len(income) > 0 {
		row.EPS = income[0].EPS
		row.Currency = income[0].Currency
	}
	if len(ratios) > 0 && ratios[0].PE != nil && *ratios[0].PE > 0 {
		row.PE = ratios[0].PE
	}
	today := time.Now().UTC().Format("2006-01-02")
	sort.Slice(estimates, func(i, j int) bool { return estimates[i].Date < estimates[j].Date })
	for _, e := range estimates {
		if e.Date >= today && e.EPS != nil && *e.EPS > 0 {
			row.ForwardEPS = e.EPS
			row.ForwardPeriod = e.Date
			break
		}
	}
	if row.ForwardEPS != nil && len(profiles) > 0 && profiles[0].Price != nil && *profiles[0].Price > 0 && row.Currency != "" && profiles[0].Currency == row.Currency {
		pe := *profiles[0].Price / *row.ForwardEPS
		row.ForwardPE = &pe
	}
	// The FMP earnings endpoint reports announcement dates. Past results are
	// never presented as the next report; future vendor dates remain estimated.
	for _, e := range earnings {
		if e.Date >= today && (row.NextEarningsDate == "" || e.Date < row.NextEarningsDate) {
			row.NextEarningsDate = e.Date
			row.EarningsDateStatus = "estimated"
		}
	}
	if row.EPS != nil || row.PE != nil || row.ForwardPE != nil || row.NextEarningsDate != "" {
		row.Status = "available"
		row.Source = "FMP（标的股票）"
		row.AsOf = time.Now().UTC().Format(time.RFC3339)
		row.Message = "公司指引 EPS 需经核验的最新财报；分析师预期仅用于远期 PE"
	}
	return row
}

func (h *StockHandler) GetFundamentals(c *gin.Context) {
	market, code := c.DefaultQuery("market", "us_stock"), c.Query("code")
	if !validStockMarket(market) || code == "" || len(code) > 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid market and code are required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, h.fundamentals.Fetch(ctx, market, code))
}
