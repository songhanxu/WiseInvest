package skill

import (
	"context"
	"encoding/json"
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
// In that case we fall back to prevClose so the context always contains a reference price.
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

		// During non-trading hours the API returns "0.00" for current price.
		// Fall back to prevClose so the LLM always has a price reference.
		preMarket := price == "" || price == "0.00"
		if preMarket {
			if prevClose == "" || prevClose == "0.00" {
				continue // truly no data
			}
			price = prevClose
			changeAmt = "-"
			changePct = "-"
		}

		sb.WriteString(fmt.Sprintf("**%s（%s）**\n", name, code))
		if preMarket {
			sb.WriteString(fmt.Sprintf("  最新收盘价：%s 元（当前未开市，昨日收盘价）\n", price))
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
// USStockPriceSkill — 美股实时行情（Yahoo Finance API）
// ─────────────────────────────────────────────

// USStockPriceSkill fetches US stock data via Yahoo Finance Chart API.
// Free, no API key required, supports NYSE and NASDAQ listed stocks.
type USStockPriceSkill struct{}

func NewUSStockPriceSkill() *USStockPriceSkill { return &USStockPriceSkill{} }

func (s *USStockPriceSkill) Name() string { return "get_us_stock_price" }

func (s *USStockPriceSkill) Description() string {
	return "查询美股（NYSE/NASDAQ）实时或最新行情数据，包括股价、涨跌幅、成交量、市值等。代码示例：AAPL（苹果）、MSFT（微软）、NVDA（英伟达）、TSLA（特斯拉）、AMZN（亚马逊）、GOOGL（谷歌）。"
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

	httpClient := &http.Client{Timeout: 10 * time.Second}
	var sb strings.Builder

	for _, sym := range strings.Split(symbols, ",") {
		sym = strings.TrimSpace(strings.ToUpper(sym))
		if sym == "" {
			continue
		}
		data, err := fetchYahooFinanceQuote(ctx, httpClient, sym)
		if err != nil {
			sb.WriteString(fmt.Sprintf("**%s**：获取数据失败（%v）\n\n", sym, err))
			continue
		}
		sb.WriteString(data)
	}

	if sb.Len() == 0 {
		return "未获取到美股数据，请检查股票代码是否正确。", nil
	}
	return sb.String(), nil
}

func fetchYahooFinanceQuote(ctx context.Context, client *http.Client, symbol string) (string, error) {
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d",
		symbol,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol              string  `json:"symbol"`
					Currency            string  `json:"currency"`
					ExchangeName        string  `json:"exchangeName"`
					RegularMarketPrice  float64 `json:"regularMarketPrice"`
					PreviousClose       float64 `json:"previousClose"`
					RegularMarketOpen   float64 `json:"regularMarketOpen"`
					RegularMarketVolume int64   `json:"regularMarketVolume"`
					MarketCap           int64   `json:"marketCap"`
				} `json:"meta"`
			} `json:"result"`
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode failed: %w", err)
	}
	if payload.Chart.Error != nil {
		return "", fmt.Errorf("API error: %s", payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) == 0 {
		return "", fmt.Errorf("no data returned")
	}

	m := payload.Chart.Result[0].Meta
	change := m.RegularMarketPrice - m.PreviousClose
	changePct := 0.0
	if m.PreviousClose > 0 {
		changePct = change / m.PreviousClose * 100
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s**（%s）\n", m.Symbol, m.ExchangeName))
	sb.WriteString(fmt.Sprintf("  当前价：%.2f %s │ 涨跌：%+.2f（%+.2f%%）\n", m.RegularMarketPrice, m.Currency, change, changePct))
	sb.WriteString(fmt.Sprintf("  开盘：%.2f │ 昨收：%.2f\n", m.RegularMarketOpen, m.PreviousClose))
	if m.RegularMarketVolume > 0 {
		sb.WriteString(fmt.Sprintf("  成交量：%d 股\n", m.RegularMarketVolume))
	}
	if m.MarketCap > 0 {
		sb.WriteString(fmt.Sprintf("  市值：%.2f 亿美元\n", float64(m.MarketCap)/1e8))
	}
	sb.WriteString("\n")
	return sb.String(), nil
}
