package handler

import (
	"context"
	"encoding/json"
	"fmt"
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
type MetricProvenance struct {
	Source   string `json:"source"`
	Currency string `json:"currency,omitempty"`
	AsOf     string `json:"as_of"`
}

type FundamentalsResponse struct {
	Metrics            map[string]MetricProvenance `json:"metrics,omitempty"`
	EPS                *float64                    `json:"eps"`
	PE                 *float64                    `json:"pe"`
	ForwardPE          *float64                    `json:"forward_pe"`
	ForwardEPS         *float64                    `json:"forward_eps"`
	ForwardPeriod      string                      `json:"forward_period,omitempty"`
	GuidanceEPS        *float64                    `json:"guidance_eps"`
	GuidanceEPSLow     *float64                    `json:"guidance_eps_low"`
	GuidanceEPSHigh    *float64                    `json:"guidance_eps_high"`
	GuidancePeriod     string                      `json:"guidance_period,omitempty"`
	GuidanceBasis      string                      `json:"guidance_basis,omitempty"`
	GuidanceReportDate string                      `json:"guidance_report_date,omitempty"`
	GuidanceSourceURL  string                      `json:"guidance_source_url,omitempty"`
	NextEarningsDate   string                      `json:"next_earnings_date,omitempty"`
	EarningsDateStatus string                      `json:"earnings_date_status,omitempty"`
	Currency           string                      `json:"currency,omitempty"`
	Source             string                      `json:"source,omitempty"`
	AsOf               string                      `json:"as_of,omitempty"`
	Status             string                      `json:"status"`
	Message            string                      `json:"message,omitempty"`
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
	pending               map[string]chan struct{}
}

func NewFundamentalsService(key, file string) *FundamentalsService {
	return &FundamentalsService{APIKey: key, File: file, BaseURL: "https://financialmodelingprep.com/stable", HTTPClient: &http.Client{Timeout: 5 * time.Second}, cache: make(map[string]fundamentalsCache), sem: make(chan struct{}, 4), pending: make(map[string]chan struct{})}
}

// Return only controlled diagnostics, never upstream bodies or URLs with keys.
func (s *FundamentalsService) get(ctx context.Context, path, symbol string, params url.Values, target interface{}) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("symbol", symbol)
	params.Set("apikey", s.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/"+path+"?"+params.Encode(), nil)
	if err != nil {
		return "请求配置无效"
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "连接失败或查询超时"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusPaymentRequired:
			return "当前 FMP 订阅无接口权限（HTTP 402）"
		case http.StatusUnauthorized:
			return "FMP 密钥校验失败（HTTP 401）"
		case http.StatusForbidden:
			return "FMP 拒绝访问（HTTP 403）"
		case http.StatusTooManyRequests:
			return "FMP 请求额度或频率受限（HTTP 429）"
		default:
			return fmt.Sprintf("FMP 接口暂不可用（HTTP %d）", resp.StatusCode)
		}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target) != nil {
		return "FMP 返回格式异常"
	}
	return ""
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

		row = s.cachedFetch(ctx, market, code)
	}
	if row.Status == "not_applicable" {
		return row
	}
	row.Metrics = make(map[string]MetricProvenance)
	for _, key := range []string{"eps", "pe", "forward_pe", "next_earnings_date"} {
		row.Metrics[key] = MetricProvenance{Source: row.Source, Currency: row.Currency, AsOf: row.AsOf}
	}

	if verified != nil {
		if verified.EPS != nil && verified.Currency != "" && (row.Currency == "" || row.Currency == verified.Currency) {
			row.EPS = verified.EPS
			row.Metrics["eps"] = MetricProvenance{Source: verified.Source, Currency: verified.Currency, AsOf: verified.AsOf}
		}
		if verified.PE != nil {
			row.PE = verified.PE
			row.Metrics["pe"] = MetricProvenance{Source: verified.Source, AsOf: verified.AsOf}
		}
		if verified.ForwardPE != nil {
			row.ForwardPE = verified.ForwardPE
			row.Metrics["forward_pe"] = MetricProvenance{Source: verified.Source, AsOf: verified.AsOf}
			row.ForwardPeriod = verified.ForwardPeriod
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
	labels := []string{"EPS", "PE", "分析师预期", "财报日期", "公司资料"}
	failures := make([]string, len(labels))
	for i, job := range []func() string{
		func() string { return s.get(ctx, "income-statement-ttm", symbol, nil, &income) },
		func() string { return s.get(ctx, "ratios-ttm", symbol, nil, &ratios) },
		func() string {
			return s.get(ctx, "analyst-estimates", symbol, url.Values{"period": {"annual"}, "limit": {"10"}}, &estimates)
		},
		func() string { return s.get(ctx, "earnings", symbol, url.Values{"limit": {"20"}}, &earnings) },
		func() string { return s.get(ctx, "profile", symbol, nil, &profiles) },
	} {
		wg.Add(1)
		go func(i int, job func() string) { defer wg.Done(); failures[i] = job() }(i, job)
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
	var diagnostics []string
	for i, failure := range failures {
		if failure != "" {
			diagnostics = append(diagnostics, labels[i]+"："+failure)
		}
	}
	if len(diagnostics) > 0 {
		row.Message = strings.Join(diagnostics, "；")
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
			taskCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			defer cancel()
			row := FundamentalsResponse{Status: "unavailable", Message: "财报查询繁忙，请稍后重试"}
			select {
			case s.sem <- struct{}{}:
				row = s.fetchFMP(taskCtx, market, code)
				<-s.sem
			case <-taskCtx.Done():
			}
			stamp := time.Now()
			if row.Status == "unavailable" {
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
