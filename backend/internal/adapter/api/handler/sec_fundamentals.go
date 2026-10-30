package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type secFundamentalsClient struct {
	baseURL, tickerURL, userAgent string
	client                        *http.Client
	mu                            sync.Mutex
	tickers                       map[string]string
	tickerExpiry, tickerRetry     time.Time
	tickerPending                 chan struct{}
	rateMu                        sync.Mutex
	nextRequest, blockedUntil     time.Time
}

func newSECFundamentalsClient(userAgent string) *secFundamentalsClient {
	return &secFundamentalsClient{baseURL: "https://data.sec.gov", tickerURL: "https://www.sec.gov/files/company_tickers.json", userAgent: strings.TrimSpace(userAgent), client: &http.Client{Timeout: 6 * time.Second}}
}

// Reserve at most four requests per second across this service, including ticker
// lookup and filings. A SEC rejection backs off across all symbols.
func (s *secFundamentalsClient) request(ctx context.Context, address, accept string) (*http.Response, string) {
	if s.userAgent == "" {
		return nil, "SEC_USER_AGENT 未配置，请提供项目名和联系邮箱"
	}
	s.rateMu.Lock()
	if time.Now().Before(s.blockedUntil) {
		s.rateMu.Unlock()
		return nil, "SEC 访问暂时受限，稍后重试"
	}
	start := time.Now()
	if start.Before(s.nextRequest) {
		start = s.nextRequest
	}
	s.nextRequest = start.Add(250 * time.Millisecond)
	s.rateMu.Unlock()
	timer := time.NewTimer(time.Until(start))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil, "SEC 查询超时"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, "SEC 请求配置无效"
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", accept)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "SEC 连接失败或查询超时"
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
			s.rateMu.Lock()
			s.blockedUntil = time.Now().Add(time.Minute)
			s.rateMu.Unlock()
		}
		return nil, fmt.Sprintf("SEC 数据暂不可用（HTTP %d）", resp.StatusCode)
	}
	return resp, ""
}

func (s *secFundamentalsClient) get(ctx context.Context, address string, target any) string {
	resp, message := s.request(ctx, address, "application/json")
	if message != "" {
		return message
	}
	defer resp.Body.Close()
	if json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(target) != nil {
		return "SEC 返回格式异常"
	}
	return ""
}

func secTickerKey(code string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), ".", "-")
}

// Cache the full official ticker/CIK mapping once daily, sharing refreshes across
// symbols. A failed refresh can use an existing mapping for up to the next retry.
func (s *secFundamentalsClient) cik(ctx context.Context, code string) (string, string) {
	for {
		s.mu.Lock()
		if time.Now().Before(s.tickerExpiry) || time.Now().Before(s.tickerRetry) {
			cik := s.tickers[secTickerKey(code)]
			s.mu.Unlock()
			if cik == "" {
				return "", "SEC 未找到该股票的 CIK 映射"
			}
			return cik, ""
		}
		if pending := s.tickerPending; pending != nil {
			s.mu.Unlock()
			select {
			case <-pending:
				continue
			case <-ctx.Done():
				return "", "SEC 代码映射查询超时"
			}
		}
		done := make(chan struct{})
		s.tickerPending = done
		s.mu.Unlock()
		var rows map[string]struct {
			CIK    int64  `json:"cik_str"`
			Ticker string `json:"ticker"`
		}
		message := s.get(ctx, s.tickerURL, &rows)
		mapping := make(map[string]string)
		for _, row := range rows {
			if row.CIK > 0 && row.Ticker != "" {
				mapping[secTickerKey(row.Ticker)] = fmt.Sprintf("%010d", row.CIK)
			}
		}
		if message == "" && len(mapping) == 0 {
			message = "SEC 代码映射为空"
		}
		s.mu.Lock()
		if message == "" {
			s.tickers, s.tickerExpiry = mapping, time.Now().Add(24*time.Hour)
		} else {
			s.tickerRetry = time.Now().Add(time.Minute)
		}
		cik := s.tickers[secTickerKey(code)]
		s.tickerPending = nil
		close(done)
		s.mu.Unlock()
		if cik != "" {
			return cik, ""
		}
		if message != "" {
			return "", message
		}
		return "", "SEC 未找到该股票的 CIK 映射"
	}
}

type secSubmissions struct {
	CIK     string   `json:"cik"`
	Tickers []string `json:"tickers"`
	Filings struct {
		Recent struct {
			Accession  []string `json:"accessionNumber"`
			Form       []string `json:"form"`
			Filed      []string `json:"filingDate"`
			ReportDate []string `json:"reportDate"`
			Document   []string `json:"primaryDocument"`
			Items      []string `json:"items"`
		} `json:"recent"`
	} `json:"filings"`
}

func (s *secFundamentalsClient) fetch(ctx context.Context, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable"}
	if s.userAgent == "" {
		row.Message = "SEC_USER_AGENT 未配置，请提供项目名和联系邮箱"
		return row
	}
	cik, message := s.cik(ctx, code)
	if message != "" {
		row.Message = message
		return row
	}
	var facts secCompanyFacts
	var submissions secSubmissions
	var factsError, filingsError string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		factsError = s.get(ctx, s.baseURL+"/api/xbrl/companyfacts/CIK"+cik+".json", &facts)
	}()
	go func() {
		defer wg.Done()
		filingsError = s.get(ctx, s.baseURL+"/submissions/CIK"+cik+".json", &submissions)
	}()
	wg.Wait()
	expected, _ := strconv.ParseInt(cik, 10, 64)
	if factsError == "" && facts.CIK != expected {
		factsError = "SEC 财务数据的 CIK 不匹配"
		facts = secCompanyFacts{}
	}
	if filingsError == "" {
		matched := false
		for _, ticker := range submissions.Tickers {
			if secTickerKey(ticker) == secTickerKey(code) {
				matched = true
			}
		}
		if submissions.CIK != cik || !matched {
			filingsError = "SEC 申报主体与股票代码不匹配"
			submissions = secSubmissions{}
		}
	}
	row.Source, row.AsOf, row.SECCIK = "SEC EDGAR（公司披露）", time.Now().UTC().Format(time.RFC3339), cik
	if filingsError == "" {
		row.FinancialReports = buildSECReports(facts, submissions, cik, time.Now())
		if release, ok := latestSECEarningsRelease(submissions, cik, time.Now()); ok && (len(row.FinancialReports) == 0 || release.FiledDate > row.FinancialReports[0].FiledDate) {
			if release, ok = s.resolveEarningsRelease(ctx, cik, release); ok && (len(row.FinancialReports) == 0 || release.ReportDate > row.FinancialReports[0].ReportDate) {
				row.FinancialReports = append([]SECFinancialReport{release}, row.FinancialReports...)
				if len(row.FinancialReports) > 8 {
					row.FinancialReports = row.FinancialReports[:8]
				}
			}
		}
	}
	// Domestic US-GAAP EPS has a known common-share basis. Foreign/ADR issuers
	// can report ordinary shares rather than ADS: preserve their statements but
	// use the market provider for EPS until the share conversion is verified.
	if factsError == "" && filingsError == "" {
		if eps, unit, basis, ok := latestSECEPS(facts, submissions, time.Now()); ok {
			value := *eps.Value
			row.EPS, row.Currency = &value, strings.TrimSuffix(unit, "/shares")
			row.EPSBasis, row.EPSPeriodStart, row.EPSPeriodEnd, row.EPSFiledDate = basis, eps.Start, eps.End, eps.Filed
			setFundamentalProvenance(&row)
			provenance := row.Metrics["eps"]
			provenance.SourceURL = secArchiveURL(cik, eps.Accession, "")
			row.Metrics["eps"] = provenance
			if trailing, start, end, complete := secTrailingEPS(facts, eps, basis, time.Now()); complete {
				row.TrailingEPS, row.TrailingEPSStart, row.TrailingEPSEnd = &trailing, start, end
				row.TrailingEPSBasis = basis + " · 最近财年"
				if secDuration(eps) < 330 {
					row.TrailingEPSBasis = "GAAP 每股收益 · TTM 估算（全年 + 本年累计 − 上年同期累计）"
				}
				row.Metrics["trailing_eps"] = provenance
			}
		}
	}
	var diagnostics []string
	for _, failure := range []string{factsError, filingsError} {
		if failure != "" {
			diagnostics = append(diagnostics, failure)
		}
	}
	if row.EPS == nil && factsError == "" {
		diagnostics = append(diagnostics, "SEC 暂无可直接用于该股票的标准披露 EPS；尝试补充来源")
	}
	row.Message = strings.Join(diagnostics, "；")
	if hasFundamentalData(row) {
		row.Status = "available"
	} else {
		row.Source, row.AsOf = "", ""
	}
	return row
}
