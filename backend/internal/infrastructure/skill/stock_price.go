package skill

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ─────────────────────────────────────────────
// ASharePriceSkill — A股实时行情（腾讯股票 API）
// ─────────────────────────────────────────────

// ASharePriceSkill fetches real-time A-share stock data via Tencent Finance API.
// Free, no authentication required. Supports Shanghai (sh), Shenzhen (sz), Beijing (bj) markets.
type ASharePriceSkill struct{}

func NewASharePriceSkill() *ASharePriceSkill { return &ASharePriceSkill{} }

func (s *ASharePriceSkill) Name() string { return "get_ashare_price" }

func (s *ASharePriceSkill) Description() string {
	return "查询A股（沪深北交所）实时股价、涨跌幅、开盘价、昨收价、成交量等行情数据。支持一次查询多只股票。代码示例：600519（茅台）、000858（五粮液）、000001（平安银行）。指数代码：sh000001（上证指数）、sz399001（深证成指）、sz399006（创业板指）。"
}

func (s *ASharePriceSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "codes",
			Type:        "string",
			Description: "股票代码，多只用英文逗号分隔。可加交易所前缀（sh/sz/bj），也可省略（系统自动判断）。例如：sh600519,sz000858 或 600519,000001",
			Required:    true,
		},
	}
}

func (s *ASharePriceSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	codes, _ := input["codes"].(string)
	if codes == "" {
		return nil, fmt.Errorf("codes is required")
	}

	normalized := normalizeAShareCodes(codes)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("no valid stock codes provided")
	}

	url := "http://qt.gtimg.cn/q=" + strings.Join(normalized, ",")
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Referer", "https://finance.qq.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stock data: %w", err)
	}
	defer resp.Body.Close()

	// Tencent Finance API responds in GBK encoding; decode to UTF-8 before parsing.
	utf8Reader := transform.NewReader(resp.Body, simplifiedchinese.GBK.NewDecoder())
	body, err := io.ReadAll(utf8Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	result := parseTencentStockResponse(string(body))
	if result == "" {
		return "未获取到股票数据，请检查股票代码是否正确。", nil
	}
	return result, nil
}

// normalizeAShareCodes auto-detects market prefix based on the first digit of the code.
// SH: starts with 6; SZ: starts with 0 or 3; BJ: starts with 4 or 8.
func normalizeAShareCodes(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, c := range parts {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "sh") || strings.HasPrefix(c, "sz") || strings.HasPrefix(c, "bj") {
			result = append(result, c)
			continue
		}
		switch {
		case strings.HasPrefix(c, "6"):
			result = append(result, "sh"+c)
		case strings.HasPrefix(c, "0") || strings.HasPrefix(c, "3"):
			result = append(result, "sz"+c)
		case strings.HasPrefix(c, "4") || strings.HasPrefix(c, "8"):
			result = append(result, "bj"+c)
		default:
			result = append(result, "sh"+c)
		}
	}
	return result
}

// parseTencentStockResponse parses the Tencent Finance API response.
// Response format per line: v_sh600519="51~贵州茅台~600519~price~prevClose~open~vol~..."
// Key field indices: [1]=name, [2]=code, [3]=price, [4]=prevClose, [5]=open, [6]=vol(手),
// [31]=changeAmt, [32]=changePct(%)
//
// During pre-market (before 9:30) or after-hours, price may be "0.00".
// In that case we use prevClose (Tencent's own last-close field) so the price source stays
// consistent — Tencent prevClose is the unadjusted last trading day's close, same series as
// the intraday price. External K-line sources use forward-adjusted (前复权) prices which can
// differ materially and would confuse the model.
func parseTencentStockResponse(raw string) string {
	var sb strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		start := strings.Index(line, "\"")
		end := strings.LastIndex(line, "\"")
		if start == -1 || end <= start {
			continue
		}
		fields := strings.Split(line[start+1:end], "~")
		if len(fields) < 33 {
			continue
		}
		name := fields[1]
		if name == "" {
			continue
		}
		code := fields[2]
		price := fields[3]
		prevClose := fields[4]
		open := fields[5]
		vol := fields[6]
		changeAmt := fields[31]
		changePct := fields[32]

		// During non-trading hours the Tencent API returns "0.00" for current price.
		// Use prevClose (same source, unadjusted) as the reference price.
		nonTrading := price == "" || price == "0.00"
		if nonTrading {
			if prevClose == "" || prevClose == "0.00" {
				continue // truly no data
			}
			price = prevClose
			changeAmt = "-"
			changePct = "-"
		}

		sb.WriteString(fmt.Sprintf("**%s（%s）**\n", name, code))
		if nonTrading {
			sb.WriteString(fmt.Sprintf("  最新收盘价（非交易时段）：%s 元\n", price))
			sb.WriteString(fmt.Sprintf("  （此为腾讯行情最近一个交易日收盘价，与今日盘中价同一数据来源）\n"))
		} else {
			sb.WriteString(fmt.Sprintf("  当前价：%s 元 │ 涨跌：%s 元（%s%%）\n", price, changeAmt, changePct))
			sb.WriteString(fmt.Sprintf("  开盘：%s 元 │ 昨收：%s 元\n", open, prevClose))
			if vol != "" && vol != "0" {
				sb.WriteString(fmt.Sprintf("  成交量：%s 手\n", vol))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ─────────────────────────────────────────────
// USStockPriceSkill — 美股实时行情（腾讯财经 API）
// ─────────────────────────────────────────────

// USStockPriceSkill fetches US stock data via Tencent Finance API.
// Free, no API key required, accessible from mainland China.
// Supports NYSE and NASDAQ listed stocks.
type USStockPriceSkill struct{}

func NewUSStockPriceSkill() *USStockPriceSkill { return &USStockPriceSkill{} }

func (s *USStockPriceSkill) Name() string { return "get_us_stock_price" }

func (s *USStockPriceSkill) Description() string {
	return "查询美股（NYSE/NASDAQ）实时或最新行情数据，包括股价、涨跌幅、成交量等。代码示例：AAPL（苹果）、MSFT（微软）、NVDA（英伟达）、TSLA（特斯拉）、AMZN（亚马逊）、GOOGL（谷歌）。"
}

func (s *USStockPriceSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "symbols",
			Type:        "string",
			Description: "美股股票代码（大写），多只用英文逗号分隔。例如：AAPL,MSFT,NVDA",
			Required:    true,
		},
	}
}

func (s *USStockPriceSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	symbols, _ := input["symbols"].(string)
	if symbols == "" {
		return nil, fmt.Errorf("symbols is required")
	}

	// Build Tencent Finance batch query codes: "usAAPL,usNVDA,usTSLA"
	var tCodes []string
	var symList []string
	for _, sym := range strings.Split(symbols, ",") {
		sym = strings.TrimSpace(strings.ToUpper(sym))
		if sym == "" {
			continue
		}
		tCodes = append(tCodes, "us"+sym)
		symList = append(symList, sym)
	}
	if len(tCodes) == 0 {
		return nil, fmt.Errorf("no valid stock symbols provided")
	}

	apiURL := fmt.Sprintf("https://qt.gtimg.cn/q=%s", strings.Join(tCodes, ","))
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stock data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	// Tencent Finance returns GBK encoding; decode to UTF-8
	utf8Body, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), body)
	if err != nil {
		utf8Body = body // fallback to raw
	}
	bodyStr := string(utf8Body)

	var sb strings.Builder
	for _, sym := range symList {
		tCode := "us" + sym
		// Parse: v_usAAPL="200~苹果~AAPL.OQ~247.99~245.00~..."
		prefix := fmt.Sprintf("v_%s=\"", tCode)
		start := strings.Index(bodyStr, prefix)
		if start == -1 {
			sb.WriteString(fmt.Sprintf("**%s**：未找到数据\n\n", sym))
			continue
		}
		start += len(prefix)
		end := strings.Index(bodyStr[start:], "\"")
		if end == -1 {
			continue
		}
		fields := strings.Split(bodyStr[start:start+end], "~")
		// Tencent US stock quote fields:
		// [0]=status [1]=中文名 [2]=code [3]=currentPrice [4]=previousClose
		// [5]=open [6]=volume [31]=change [32]=changePct [33]=high [34]=low
		// [44]=English name
		if len(fields) < 35 {
			sb.WriteString(fmt.Sprintf("**%s**：数据字段不完整\n\n", sym))
			continue
		}

		chineseName := fields[1]
		currentPrice := fields[3]
		previousClose := fields[4]
		openPrice := fields[5]
		volume := fields[6]
		change := fields[31]
		changePct := fields[32]
		high := fields[33]
		low := fields[34]
		engName := ""
		if len(fields) > 44 {
			engName = fields[44]
		}

		displayName := chineseName
		if displayName == "" {
			displayName = engName
		}
		if displayName == "" {
			displayName = sym
		}

		// During non-trading hours price may be "0.00"
		nonTrading := currentPrice == "" || currentPrice == "0.00"
		if nonTrading {
			if previousClose == "" || previousClose == "0.00" {
				continue
			}
			currentPrice = previousClose
			change = "-"
			changePct = "-"
		}

		sb.WriteString(fmt.Sprintf("**%s（%s）**\n", displayName, sym))
		if nonTrading {
			sb.WriteString(fmt.Sprintf("  最新收盘价（非交易时段）：%s USD\n", currentPrice))
		} else {
			sb.WriteString(fmt.Sprintf("  当前价：%s USD │ 涨跌：%s（%s%%）\n", currentPrice, change, changePct))
			sb.WriteString(fmt.Sprintf("  开盘：%s │ 昨收：%s\n", openPrice, previousClose))
			sb.WriteString(fmt.Sprintf("  最高：%s │ 最低：%s\n", high, low))
			if volume != "" && volume != "0" {
				sb.WriteString(fmt.Sprintf("  成交量：%s 股\n", volume))
			}
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		return "未获取到美股数据，请检查股票代码是否正确。", nil
	}
	return sb.String(), nil
}

