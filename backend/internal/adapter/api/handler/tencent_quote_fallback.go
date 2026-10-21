package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/bitget"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func tencentQuoteTime(raw, market string) string {
	zone := "Asia/Shanghai"
	if market == "us_stock" {
		zone = "America/New_York"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return ""
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006/01/02 15:04:05", "20060102150405"} {
		if stamp, err := time.ParseInLocation(layout, strings.TrimSpace(raw), location); err == nil {
			return stamp.Format(time.RFC3339)
		}
	}
	return ""
}

func tencentQuoteSession(raw, market string) string {
	stamp, err := time.Parse(time.RFC3339, tencentQuoteTime(raw, market))
	if err != nil {
		return ""
	}
	if market == "us_stock" && stamp.Hour() == 16 && stamp.Minute() == 0 {
		return "close"
	}
	return "regular"
}

func numericHKCode(code string) bool {
	if len(code) != 5 || code == "00000" {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func (h *StockHandler) tencentPublicText(ctx context.Context, address string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Tencent quotes HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), body)
	return string(decoded), err
}

func (h *StockHandler) fetchTencentHKStocks(ctx context.Context, codes []string) ([]StockResponse, error) {
	var symbols []string
	seen := map[string]bool{}
	for _, raw := range codes {
		code := bitget.NormalizeCode("hk_stock", raw)
		if numericHKCode(code) && !seen[code] {
			symbols = append(symbols, "hk"+code)
			seen[code] = true
		}
	}
	if len(symbols) == 0 {
		return []StockResponse{}, nil
	}
	body, err := h.tencentPublicText(ctx, "https://qt.gtimg.cn/q="+strings.Join(symbols, ","))
	if err != nil {
		return nil, err
	}
	var rows []StockResponse
	for _, symbol := range symbols {
		_, rest, found := strings.Cut(body, "v_"+symbol+"=\"")
		if !found {
			continue
		}
		text, _, _ := strings.Cut(rest, "\"")
		fields := strings.Split(text, "~")
		if len(fields) < 35 || parseFloat(fields[3]) <= 0 {
			continue
		}
		code := strings.TrimPrefix(symbol, "hk")
		rows = append(rows, StockResponse{ID: code, Symbol: code, Name: fields[1], Market: "hk_stock", CurrentPrice: parseFloat(fields[3]), PreviousClose: parseFloat(fields[4]), Open: parseFloat(fields[5]), Volume: parseFloat(fields[6]) / 1e8, Change: parseFloat(fields[31]), ChangePercent: parseFloat(fields[32]), High: parseFloat(fields[33]), Low: parseFloat(fields[34]), Source: "tencent", Currency: "HKD", ProductType: "equity", ChangeBasis: "previous_close", QuoteTime: tencentQuoteTime(fields[30], "hk_stock"), QuoteSession: tencentQuoteSession(fields[30], "hk_stock")})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Hong Kong stock quotes unavailable")
	}
	return rows, nil
}

func (h *StockHandler) searchTencentHKStocks(ctx context.Context, query string) ([]StockResponse, error) {
	if strings.TrimSpace(query) == "" {
		return h.fetchTencentHKStocks(ctx, []string{"00700", "01810", "00992", "09988"})
	}
	if code := bitget.NormalizeCode("hk_stock", query); numericHKCode(code) {
		return h.fetchTencentHKStocks(ctx, []string{code})
	}
	body, err := h.tencentPublicText(ctx, "https://smartbox.gtimg.cn/s3/?q="+url.QueryEscape(query)+"&t=hk")
	if err != nil {
		return nil, err
	}
	_, body, found := strings.Cut(body, "\"")
	if !found {
		return []StockResponse{}, nil
	}
	body, _, _ = strings.Cut(body, "\"")
	var codes []string
	for _, entry := range strings.Split(body, "^") {
		fields := strings.Split(entry, "~")
		if len(fields) >= 2 && fields[0] == "hk" && len(codes) < 10 {
			codes = append(codes, fields[1])
		}
	}
	return h.fetchTencentHKStocks(ctx, codes)
}
