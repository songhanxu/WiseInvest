package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Yahoo wraps numbers in {raw, fmt} unless formatted=false is honored. A null
// value remains nil, including zero EPS and loss-making companies.
type yahooNumber struct {
	Raw *float64 `json:"raw"`
}

func (n *yahooNumber) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '{' {
		type plain yahooNumber
		return json.Unmarshal(data, (*plain)(n))
	}
	return json.Unmarshal(data, &n.Raw)
}

type yahooFundamentalsClient struct {
	baseURL, cookieURL, crumbURL string
	client                       *http.Client
	gate                         chan struct{}
	crumb                        string
	expires, retryAt             time.Time
}

func newYahooFundamentalsClient() *yahooFundamentalsClient {
	jar, _ := cookiejar.New(nil)
	return &yahooFundamentalsClient{baseURL: "https://query2.finance.yahoo.com", cookieURL: "https://fc.yahoo.com", crumbURL: "https://query1.finance.yahoo.com/v1/test/getcrumb", client: &http.Client{Timeout: 5 * time.Second, Jar: jar}, gate: make(chan struct{}, 1)}
}

func yahooStockSymbol(market, code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	switch market {
	case "us_stock":
		return strings.ReplaceAll(code, ".", "-")
	case "hk_stock":
		code = strings.TrimSuffix(strings.TrimPrefix(code, "HK"), ".HK")
		code = strings.TrimLeft(code, "0")
		for len(code) < 4 {
			code = "0" + code
		}
		return code + ".HK"
	case "a_share":
		code = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(code, "SH"), "SZ"), "BJ")
		if strings.Contains(code, ".") {
			return code
		}
		if strings.HasPrefix(code, "6") {
			return code + ".SS"
		}
		if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92") {
			return code + ".BJ"
		}
		return code + ".SZ"
	default:
		return code
	}
}

func (y *yahooFundamentalsClient) request(ctx context.Context, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	return y.client.Do(req)
}

// One cookie/crumb refresh is shared by all symbols. Waiting callers can cancel,
// and auth failures back off instead of requesting a new cookie for every row.
func (y *yahooFundamentalsClient) credentials(ctx context.Context, rejected string) (string, string) {
	select {
	case y.gate <- struct{}{}:
	case <-ctx.Done():
		return "", "Yahoo Finance 查询超时"
	}
	defer func() { <-y.gate }()
	if rejected != "" && y.crumb == rejected {
		y.crumb = ""
	}
	if y.crumb != "" && time.Now().Before(y.expires) {
		return y.crumb, ""
	}
	if time.Now().Before(y.retryAt) {
		return "", "Yahoo Finance 会话暂不可用，稍后重试"
	}
	y.retryAt = time.Now().Add(time.Minute)
	resp, err := y.request(ctx, y.cookieURL)
	if err != nil {
		return "", "Yahoo Finance 会话连接失败或查询超时"
	}
	// fc.yahoo.com commonly returns 404 while still setting the required cookie.
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return "", fmt.Sprintf("Yahoo Finance 会话暂不可用（HTTP %d）", resp.StatusCode)
	}
	resp, err = y.request(ctx, y.crumbURL)
	if err != nil {
		return "", "Yahoo Finance 会话校验连接失败或查询超时"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("Yahoo Finance 会话校验失败（HTTP %d）", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	crumb := strings.TrimSpace(string(data))
	if err != nil || crumb == "" || len(crumb) > 128 || strings.ContainsAny(crumb, "<>\r\n ") {
		return "", "Yahoo Finance 会话校验返回异常"
	}
	y.crumb, y.expires, y.retryAt = crumb, time.Now().Add(6*time.Hour), time.Time{}
	return crumb, ""
}

func (y *yahooFundamentalsClient) fetch(ctx context.Context, market, code string) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable"}
	rejected := ""
	for attempt := 0; attempt < 2; attempt++ {
		crumb, message := y.credentials(ctx, rejected)
		if message != "" {
			row.Message = message
			return row
		}
		params := url.Values{"modules": {"price,summaryDetail,defaultKeyStatistics,financialData,calendarEvents,assetProfile"}, "formatted": {"false"}, "crumb": {crumb}}
		address := y.baseURL + "/v10/finance/quoteSummary/" + url.PathEscape(yahooStockSymbol(market, code)) + "?" + params.Encode()
		resp, err := y.request(ctx, address)
		if err != nil {
			row.Message = "Yahoo Finance 连接失败或查询超时"
			return row
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			resp.Body.Close()
			rejected = crumb
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			row.Message = fmt.Sprintf("Yahoo Finance 基本面暂不可用（HTTP %d）", resp.StatusCode)
			return row
		}
		row = decodeYahooFundamentals(io.LimitReader(resp.Body, 2<<20), time.Now())
		resp.Body.Close()
		return row
	}
	return row
}

func decodeYahooFundamentals(body io.Reader, now time.Time) FundamentalsResponse {
	row := FundamentalsResponse{Status: "unavailable"}
	var data struct {
		QuoteSummary struct {
			Error  json.RawMessage `json:"error"`
			Result []struct {
				Price   struct{ Symbol, Currency, QuoteType, ExchangeTimezoneName string } `json:"price"`
				Summary struct {
					PE        yahooNumber `json:"trailingPE"`
					ForwardPE yahooNumber `json:"forwardPE"`
				} `json:"summaryDetail"`
				Stats struct {
					EPS        yahooNumber `json:"trailingEps"`
					ForwardEPS yahooNumber `json:"forwardEps"`
					ForwardPE  yahooNumber `json:"forwardPE"`
				} `json:"defaultKeyStatistics"`
				Financial struct {
					Currency string `json:"financialCurrency"`
				} `json:"financialData"`
				Profile struct {
					Description string `json:"longBusinessSummary"`
				} `json:"assetProfile"`
				Calendar struct {
					Earnings struct {
						Dates []yahooNumber `json:"earningsDate"`
					} `json:"earnings"`
				} `json:"calendarEvents"`
			} `json:"result"`
		} `json:"quoteSummary"`
	}
	if json.NewDecoder(body).Decode(&data) != nil || len(data.QuoteSummary.Result) != 1 || (len(data.QuoteSummary.Error) > 0 && string(data.QuoteSummary.Error) != "null") {
		row.Message = "Yahoo Finance 未返回有效基本面数据"
		return row
	}
	item := data.QuoteSummary.Result[0]
	if item.Price.QuoteType == "ETF" || item.Price.QuoteType == "MUTUALFUND" || item.Price.QuoteType == "INDEX" {
		row.Status, row.Message = "not_applicable", "ETF / 基金 / 指数不适用公司 EPS 与财报指引"
		return row
	}
	row.Source, row.AsOf = "Yahoo Finance（标的股票）", now.UTC().Format(time.RFC3339)
	row.Description = strings.TrimSpace(item.Profile.Description)
	// The financial reporting currency can differ from the stock's quote currency.
	// Without an explicit financial currency, leave EPS unavailable.
	row.Currency = item.Financial.Currency
	if row.Currency != "" {
		row.EPS, row.ForwardEPS = item.Stats.EPS.Raw, item.Stats.ForwardEPS.Raw
		if row.EPS != nil {
			row.EPSBasis = "TTM（供应商）"
		}
	}
	if pe := item.Summary.PE.Raw; pe != nil && *pe > 0 {
		row.PE = pe
	}
	forwardPE := item.Stats.ForwardPE.Raw
	if forwardPE == nil {
		forwardPE = item.Summary.ForwardPE.Raw
	}
	if forwardPE != nil && *forwardPE > 0 {
		row.ForwardPE = forwardPE
	}
	if row.ForwardPE != nil || row.ForwardEPS != nil {
		row.ForwardBasis = "分析师预期；供应商未提供年度期末"
	}
	location := time.UTC
	if zone, err := time.LoadLocation(item.Price.ExchangeTimezoneName); err == nil {
		location = zone
	}
	today := now.In(location).Format("2006-01-02")
	for _, raw := range item.Calendar.Earnings.Dates {
		if raw.Raw == nil || *raw.Raw <= 0 {
			continue
		}
		date := time.Unix(int64(*raw.Raw), 0).In(location).Format("2006-01-02")
		if date < today {
			continue
		}
		if row.NextEarningsDate == "" || date < row.NextEarningsDate {
			row.NextEarningsDate = date
		}
		if date > row.NextEarningsDateEnd {
			row.NextEarningsDateEnd = date
		}
	}
	if row.NextEarningsDateEnd == row.NextEarningsDate {
		row.NextEarningsDateEnd = ""
	}
	if row.NextEarningsDate != "" {
		row.EarningsDateStatus = "estimated"
	}
	if hasFundamentalData(row) {
		row.Status = "available"
	} else {
		row.Message = "Yahoo Finance 暂无该标的的 EPS、估值或未来财报日期"
	}
	setFundamentalProvenance(&row)
	return row
}
