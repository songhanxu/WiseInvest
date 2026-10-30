package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

func hasFundamentalData(row FundamentalsResponse) bool {
	return row.EPS != nil || row.PE != nil || row.ForwardPE != nil || row.ForwardEPS != nil || row.NextEarningsDate != "" || len(row.FinancialReports) > 0
}

func setFundamentalProvenance(row *FundamentalsResponse) {
	row.Metrics = make(map[string]MetricProvenance)
	for key, present := range map[string]bool{"eps": row.EPS != nil, "pe": row.PE != nil, "forward_pe": row.ForwardPE != nil, "forward_eps": row.ForwardEPS != nil, "next_earnings_date": row.NextEarningsDate != ""} {
		if present {
			currency := ""
			if key == "eps" || key == "forward_eps" {
				currency = row.Currency
			}
			row.Metrics[key] = MetricProvenance{Source: row.Source, Currency: currency, AsOf: row.AsOf}
		}
	}
}

// Fill fields independently. Annual estimates and their valuation form one
// group; a PE from one provider must not be paired with another provider's EPS.
func mergeFundamentals(row *FundamentalsResponse, extra FundamentalsResponse) {
	if row.Metrics == nil {
		row.Metrics = make(map[string]MetricProvenance)
	}
	used := false
	copyMetric := func(key string) { row.Metrics[key] = extra.Metrics[key]; used = true }
	if row.Description == "" {
		row.Description = extra.Description
	}
	if row.EPS == nil && extra.EPS != nil {
		row.EPS, row.Currency = extra.EPS, extra.Currency
		row.EPSBasis, row.EPSPeriodStart, row.EPSPeriodEnd, row.EPSFiledDate = extra.EPSBasis, extra.EPSPeriodStart, extra.EPSPeriodEnd, extra.EPSFiledDate
		copyMetric("eps")
	}
	if row.PE == nil && extra.PE != nil {
		row.PE = extra.PE
		copyMetric("pe")
	}
	if row.ForwardPE == nil && extra.ForwardPE != nil && row.ForwardEPS == nil {
		row.ForwardPE, row.ForwardEPS = extra.ForwardPE, extra.ForwardEPS
		row.ForwardPeriod, row.ForwardBasis = extra.ForwardPeriod, extra.ForwardBasis
		copyMetric("forward_pe")
		if extra.ForwardEPS != nil {
			copyMetric("forward_eps")
		}
	} else if row.ForwardPE == nil && row.ForwardEPS == nil && extra.ForwardEPS != nil {
		row.ForwardEPS, row.ForwardPeriod, row.ForwardBasis = extra.ForwardEPS, extra.ForwardPeriod, extra.ForwardBasis
		copyMetric("forward_eps")
	}
	if row.NextEarningsDate == "" && extra.NextEarningsDate != "" {
		row.NextEarningsDate, row.NextEarningsDateEnd = extra.NextEarningsDate, extra.NextEarningsDateEnd
		row.EarningsDateStatus = extra.EarningsDateStatus
		copyMetric("next_earnings_date")
	}
	if used {
		if row.Source == "" {
			row.Source = extra.Source
		} else if extra.Source != "" {
			row.Source += " + " + extra.Source
		}
		if row.AsOf == "" {
			row.AsOf = extra.AsOf
		}
	}
}

func (s *FundamentalsService) fetchSources(ctx context.Context, market, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable"}
	var sec, yahoo, finnhub FundamentalsResponse
	var wg sync.WaitGroup
	if s.SEC != nil && market == "us_stock" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			secCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			sec = s.secResult(secCtx, code)
		}()
	}
	if s.Yahoo != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			yahooCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			yahoo = s.sourceResult(yahooCtx, "yahoo:"+market+":"+code, market+":"+code, 6*time.Hour,
				func(fetchCtx context.Context) FundamentalsResponse { return s.Yahoo.fetch(fetchCtx, market, code) })
		}()
	}
	if s.FinnhubAPIKey != "" && market == "us_stock" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			finnhub = s.sourceResult(ctx, "finnhub:"+market+":"+code, market+":"+code, time.Hour,
				func(fetchCtx context.Context) FundamentalsResponse { return s.fetchFinnhubCalendar(fetchCtx, code) })
		}()
	}
	wg.Wait()
	if yahoo.Status == "not_applicable" && !hasFundamentalData(sec) {
		return yahoo
	}
	row = sec
	if row.Status == "" {
		row.Status = "unavailable"
	}
	mergeFundamentals(&row, yahoo)
	mergeFundamentals(&row, finnhub)
	var messages []string
	for _, message := range []string{sec.Message, yahoo.Message, finnhub.Message} {
		if message != "" {
			messages = append(messages, message)
		}
	}
	row.Message = strings.Join(messages, "；")
	if hasFundamentalData(row) {
		row.Status = "available"
	} else if row.Message == "" {
		row.Message = "未取得基本面数据；请检查 SEC、Yahoo Finance 与 Finnhub 配置"
	}
	return row
}

func (s *FundamentalsService) fetchFinnhubCalendar(ctx context.Context, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable"}
	today := time.Now().UTC()
	params := url.Values{"symbol": {code}, "from": {today.Format("2006-01-02")}, "to": {today.AddDate(0, 4, 0).Format("2006-01-02")}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.FinnhubBaseURL+"/calendar/earnings?"+params.Encode(), nil)
	if err != nil {
		row.Message = "Finnhub 请求配置无效"
		return row
	}
	// Keep the credential out of request URLs and upstream diagnostics.
	req.Header.Set("X-Finnhub-Token", s.FinnhubAPIKey)
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		row.Message = "Finnhub 财报日历连接失败或查询超时"
		return row
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		row.Message = fmt.Sprintf("Finnhub 财报日历暂不可用（HTTP %d）", resp.StatusCode)
		return row
	}
	var data struct {
		Error    string `json:"error"`
		Calendar []struct {
			Date   string `json:"date"`
			Symbol string `json:"symbol"`
		} `json:"earningsCalendar"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&data) != nil || data.Error != "" {
		row.Message = "Finnhub 财报日历返回异常"
		return row
	}
	for _, event := range data.Calendar {
		_, err := time.Parse("2006-01-02", event.Date)
		if err == nil && event.Symbol == code && event.Date >= params.Get("from") && event.Date <= params.Get("to") && (row.NextEarningsDate == "" || event.Date < row.NextEarningsDate) {
			row.NextEarningsDate = event.Date
		}
	}
	if row.NextEarningsDate != "" {
		row.Status, row.Source, row.AsOf = "available", "Finnhub（财报日历）", today.Format(time.RFC3339)
		row.EarningsDateStatus = "estimated"
	} else {
		row.Message = "Finnhub 暂无未来四个月的财报日期"
	}
	setFundamentalProvenance(&row)
	return row
}
