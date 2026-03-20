package skill

// ashare_market.go provides two A-share specific skills using the Eastmoney public API:
//   - AShareSectorSkill:       today's sector (board) performance ranking
//   - AShareStockDetailSkill:  individual stock fundamental data (PE, PB, market cap, etc.)
//
// Both APIs are free and require no authentication.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// AShareSectorSkill — 板块涨跌排行（东方财富板块 API）
// ─────────────────────────────────────────────────────────────────────────────

// AShareSectorSkill fetches today's A-share sector performance rankings.
// It uses Eastmoney's public board list API (no auth required).
//
// Supported board types:
//   - "行业" (industry): 申万/中信行业分类板块
//   - "概念" (concept): 热门概念板块
type AShareSectorSkill struct{}

func NewAShareSectorSkill() *AShareSectorSkill { return &AShareSectorSkill{} }

func (s *AShareSectorSkill) Name() string { return "get_ashare_sectors" }

func (s *AShareSectorSkill) Description() string {
	return "查询A股今日板块涨跌排行榜，包括各行业板块和概念板块的涨跌幅、成交额、领涨股等。可按行业板块或概念板块分类查询，适合分析热点板块轮动和资金流向。"
}

func (s *AShareSectorSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "type",
			Type:        "string",
			Description: "板块类型：\"行业\" 查询申万行业板块，\"概念\" 查询热门概念板块。默认为 \"行业\"",
			Required:    false,
			Enum:        []string{"行业", "概念"},
		},
		{
			Name:        "top_n",
			Type:        "integer",
			Description: "返回涨幅最高和跌幅最大的各 N 个板块，默认 10，最多 20",
			Required:    false,
		},
	}
}

func (s *AShareSectorSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	boardType := "行业"
	if t, ok := input["type"].(string); ok && t != "" {
		boardType = t
	}

	topN := 10
	if n, ok := input["top_n"]; ok {
		switch v := n.(type) {
		case float64:
			topN = int(v)
		case int:
			topN = v
		}
	}
	if topN < 1 {
		topN = 5
	}
	if topN > 20 {
		topN = 20
	}

	// Eastmoney board type codes:
	//   t:2 = 行业板块 (industry)
	//   t:3 = 概念板块 (concept)
	typeCode := "2"
	if boardType == "概念" {
		typeCode = "3"
	}

	url := fmt.Sprintf(
		"https://push2.eastmoney.com/api/qt/clist/get?pn=1&pz=200&po=1&np=1&fltt=2&invt=2&fid=f3&fs=m:90+t:%s+f:%%2150&fields=f3,f4,f6,f12,f14,f128,f140&ut=bd1d9ddb04089700cf9c27f6f7426281",
		typeCode,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Referer", "https://www.eastmoney.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sector data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Data struct {
			Total int `json:"total"`
			Diff  []struct {
				ChangePct   float64 `json:"f3"`  // 涨跌幅 (%)
				ChangeAmt   float64 `json:"f4"`  // 涨跌额
				Amount      float64 `json:"f6"`  // 成交额 (元)
				Code        string  `json:"f12"` // 板块代码
				Name        string  `json:"f14"` // 板块名称
				LeadStock   string  `json:"f128"` // 领涨股
				LeadChgPct  float64 `json:"f140"` // 领涨股涨跌幅
			} `json:"diff"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	sectors := result.Data.Diff
	if len(sectors) == 0 {
		return "暂无板块数据，可能当前非交易时段。", nil
	}

	// Sort ascending to get losers easily
	sorted := make([]struct {
		ChangePct  float64
		ChangeAmt  float64
		Amount     float64
		Name       string
		LeadStock  string
		LeadChgPct float64
	}, len(sectors))
	for i, s := range sectors {
		sorted[i].ChangePct = s.ChangePct
		sorted[i].ChangeAmt = s.ChangeAmt
		sorted[i].Amount = s.Amount
		sorted[i].Name = s.Name
		sorted[i].LeadStock = s.LeadStock
		sorted[i].LeadChgPct = s.LeadChgPct
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ChangePct > sorted[j].ChangePct
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## A股%s板块涨跌榜（共 %d 个板块）\n\n", boardType, len(sectors)))

	// Top gainers
	gainers := sorted
	if len(gainers) > topN {
		gainers = gainers[:topN]
	}
	sb.WriteString(fmt.Sprintf("### 🔴 涨幅前 %d 板块\n", len(gainers)))
	sb.WriteString("| 板块 | 涨跌幅 | 成交额 | 领涨股 |\n")
	sb.WriteString("|------|--------|--------|--------|\n")
	for _, sec := range gainers {
		amtStr := formatAmount(sec.Amount)
		leadStr := ""
		if sec.LeadStock != "" {
			leadStr = fmt.Sprintf("%s（%+.2f%%）", sec.LeadStock, sec.LeadChgPct)
		}
		sb.WriteString(fmt.Sprintf("| %s | %+.2f%% | %s | %s |\n",
			sec.Name, sec.ChangePct, amtStr, leadStr))
	}

	sb.WriteString("\n")

	// Bottom losers
	losers := make([]struct {
		ChangePct  float64
		ChangeAmt  float64
		Amount     float64
		Name       string
		LeadStock  string
		LeadChgPct float64
	}, len(sorted))
	copy(losers, sorted)
	sort.Slice(losers, func(i, j int) bool {
		return losers[i].ChangePct < losers[j].ChangePct
	})
	showLosers := topN / 2
	if showLosers < 3 {
		showLosers = 3
	}
	if len(losers) > showLosers {
		losers = losers[:showLosers]
	}
	sb.WriteString(fmt.Sprintf("### 🟢 跌幅前 %d 板块\n", len(losers)))
	sb.WriteString("| 板块 | 涨跌幅 | 成交额 | 领跌股 |\n")
	sb.WriteString("|------|--------|--------|--------|\n")
	for _, sec := range losers {
		amtStr := formatAmount(sec.Amount)
		leadStr := ""
		if sec.LeadStock != "" {
			leadStr = fmt.Sprintf("%s（%+.2f%%）", sec.LeadStock, sec.LeadChgPct)
		}
		sb.WriteString(fmt.Sprintf("| %s | %+.2f%% | %s | %s |\n",
			sec.Name, sec.ChangePct, amtStr, leadStr))
	}

	return sb.String(), nil
}

// formatAmount formats a float amount in 元 to a human-readable string (亿/万).
func formatAmount(amount float64) string {
	if amount <= 0 {
		return "-"
	}
	if amount >= 1e8 {
		return fmt.Sprintf("%.2f亿", amount/1e8)
	}
	return fmt.Sprintf("%.2f万", amount/1e4)
}

// ─────────────────────────────────────────────────────────────────────────────
// AShareStockDetailSkill — 个股基本面（东方财富行情 API）
// ─────────────────────────────────────────────────────────────────────────────

// AShareStockDetailSkill fetches fundamental data for individual A-share stocks.
// Data includes PE, PB, market cap, turnover rate, 52-week range, etc.
// Uses Eastmoney's stock detail API (free, no auth required).
type AShareStockDetailSkill struct{}

func NewAShareStockDetailSkill() *AShareStockDetailSkill { return &AShareStockDetailSkill{} }

func (s *AShareStockDetailSkill) Name() string { return "get_ashare_fundamentals" }

func (s *AShareStockDetailSkill) Description() string {
	return "查询A股个股的基本面数据，包括：市盈率PE(TTM)、市净率PB、总市值、流通市值、换手率、52周高低点等。适合做估值分析时使用。可一次查询多只股票。"
}

func (s *AShareStockDetailSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "codes",
			Type:        "string",
			Description: "股票代码，多只用英文逗号分隔。可省略交易所前缀，系统自动识别。例如：600519,000858,300750",
			Required:    true,
		},
	}
}

func (s *AShareStockDetailSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	codes, _ := input["codes"].(string)
	if codes == "" {
		return nil, fmt.Errorf("codes is required")
	}

	normalized := normalizeAShareCodes(codes)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("no valid stock codes provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var sb strings.Builder

	for _, code := range normalized {
		data, err := fetchEastmoneyStockDetail(ctx, client, code)
		if err != nil {
			sb.WriteString(fmt.Sprintf("**%s**：获取数据失败（%v）\n\n", code, err))
			continue
		}
		sb.WriteString(data)
	}

	if sb.Len() == 0 {
		return "未获取到数据，请检查股票代码是否正确。", nil
	}
	return sb.String(), nil
}

// fetchEastmoneyStockDetail fetches fundamental data for a single stock from Eastmoney.
// The secid format is "1.600519" for SH, "0.000858" for SZ.
func fetchEastmoneyStockDetail(ctx context.Context, client *http.Client, code string) (string, error) {
	// Convert sh/sz prefix to Eastmoney secid prefix (1=SH, 0=SZ, 0=BJ)
	secPrefix := "1"
	rawCode := code
	if strings.HasPrefix(code, "sh") {
		secPrefix = "1"
		rawCode = code[2:]
	} else if strings.HasPrefix(code, "sz") {
		secPrefix = "0"
		rawCode = code[2:]
	} else if strings.HasPrefix(code, "bj") {
		secPrefix = "0"
		rawCode = code[2:]
	}

	// Eastmoney stock quote API with fundamental fields:
	// f43=current price, f60=prev close, f57=code, f58=name,
	// f116=circulation market cap, f117=total market cap,
	// f162=PE(TTM), f167=PB,
	// f168=turnover rate, f169=change amount, f170=change %,
	// f114=52w high, f115=52w low
	url := fmt.Sprintf(
		"https://push2.eastmoney.com/api/qt/stock/get?ut=b2884a393a59ad64002292a3e90d46a5&invt=2&fltt=2&fields=f43,f60,f57,f58,f116,f117,f162,f163,f164,f165,f167,f168,f169,f170,f114,f115&secid=%s.%s",
		secPrefix, rawCode,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "https://www.eastmoney.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Price          interface{} `json:"f43"`  // 最新价（盘中实时；非交易时段可能为0或"-"）
			PrevClose      interface{} `json:"f60"`  // 昨日收盘价（始终有值，作为价格兜底）
			Code           string      `json:"f57"`
			Name           string      `json:"f58"`
			CirculationCap interface{} `json:"f116"` // 流通市值
			TotalCap       interface{} `json:"f117"` // 总市值
			PE             interface{} `json:"f162"` // PE (TTM)
			PB             interface{} `json:"f167"` // PB
			TurnoverRate   interface{} `json:"f168"` // 换手率 (%)
			ChangeAmt      interface{} `json:"f169"` // 涨跌额
			ChangePct      interface{} `json:"f170"` // 涨跌幅 (%)
			High52W        interface{} `json:"f114"` // 52周高
			Low52W         interface{} `json:"f115"` // 52周低
		} `json:"data"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse failed: %w", err)
	}

	d := result.Data
	if d.Name == "" || d.Name == "-" {
		return "", fmt.Errorf("stock not found")
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("**%s（%s）** 基本面数据\n", d.Name, d.Code))
	// Note: current price is intentionally omitted here — it is always provided
	// by the get_ashare_price skill (real-time Tencent API) which is more reliable.
	// Including price here creates conflicts when Eastmoney f43 is stale or zero.

	// Valuation
	if pe := toFloat(d.PE); pe != 0 {
		out.WriteString(fmt.Sprintf("  PE（TTM）：%.2f 倍", pe))
	} else {
		out.WriteString("  PE（TTM）：-")
	}
	if pb := toFloat(d.PB); pb > 0 {
		out.WriteString(fmt.Sprintf(" │ PB：%.2f 倍\n", pb))
	} else {
		out.WriteString(" │ PB：-\n")
	}

	// Market cap
	if totalCap := toFloat(d.TotalCap); totalCap > 0 {
		out.WriteString(fmt.Sprintf("  总市值：%s", formatCapital(totalCap)))
	}
	if circCap := toFloat(d.CirculationCap); circCap > 0 {
		out.WriteString(fmt.Sprintf(" │ 流通市值：%s\n", formatCapital(circCap)))
	} else {
		out.WriteString("\n")
	}

	// Turnover rate & 52w range
	if tr := toFloat(d.TurnoverRate); tr > 0 {
		out.WriteString(fmt.Sprintf("  换手率：%.2f%%\n", tr))
	}
	h52 := toFloat(d.High52W)
	l52 := toFloat(d.Low52W)
	if h52 > 0 && l52 > 0 {
		out.WriteString(fmt.Sprintf("  52周区间：%.2f ~ %.2f 元\n", l52, h52))
	}
	out.WriteString("\n")

	return out.String(), nil
}

// toFloat safely converts interface{} (which may be float64 or string "-") to float64.
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	}
	return 0
}

// formatCapital formats a market cap value (in 元) to 亿/万 string.
func formatCapital(yuan float64) string {
	if yuan >= 1e12 {
		return fmt.Sprintf("%.2f 万亿", yuan/1e12)
	}
	if yuan >= 1e8 {
		return fmt.Sprintf("%.2f 亿", yuan/1e8)
	}
	return fmt.Sprintf("%.2f 万", yuan/1e4)
}

// ─────────────────────────────────────────────────────────────────────────────
// LookupAShareCodeSkill — 股票名称搜索代码（东方财富搜索 API）
// ─────────────────────────────────────────────────────────────────────────────

// LookupAShareCodeSkill resolves a Chinese A-share stock name or keyword to its
// 6-digit stock code, using Eastmoney's public search API (no auth required).
type LookupAShareCodeSkill struct{}

func NewLookupAShareCodeSkill() *LookupAShareCodeSkill { return &LookupAShareCodeSkill{} }

func (s *LookupAShareCodeSkill) Name() string { return "lookup_ashare_code" }

func (s *LookupAShareCodeSkill) Description() string {
	return "通过股票名称或公司名称搜索A股股票代码。当用户只提供了股票名称（如'贵州茅台'、'特变电工'）而没有股票代码时，使用此工具查找对应的6位代码，然后再用代码查询实时行情和基本面数据。"
}

func (s *LookupAShareCodeSkill) Parameters() []SkillParam {
	return []SkillParam{
		{Name: "name", Type: "string", Description: "股票名称、公司简称或关键词，例如：特变电工、贵州茅台、宁德时代", Required: true},
	}
}

func (s *LookupAShareCodeSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	results, err := SearchAShareByName(ctx, name)
	if err != nil || len(results) == 0 {
		return fmt.Sprintf("未找到名称为'%s'的A股股票", name), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("'%s' 的A股搜索结果：\n", name))
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("- %s，代码：%s（%s）\n", r.Name, r.Code, r.Market))
	}
	return sb.String(), nil
}

// AShareSearchResult represents one result from the Eastmoney stock name search.
type AShareSearchResult struct {
	Code   string // 6-digit code, e.g. "000401"
	Name   string // stock short name, e.g. "特变电工"
	Market string // "上交所" or "深交所"
}

// SearchAShareByName queries Eastmoney's suggest API to resolve a stock name to its code.
// Returns the first few matches sorted by relevance.
func SearchAShareByName(ctx context.Context, name string) ([]AShareSearchResult, error) {
	// Eastmoney public suggest API — free, no auth required.
	// url.QueryEscape is required: Chinese characters must be percent-encoded or the
	// server will return a non-JSON response and the lookup silently fails.
	apiURL := "https://searchapi.eastmoney.com/api/suggest/get?input=" + url.QueryEscape(name) +
		"&type=14&count=5&markettype=&mktnum=&jys=&classify=&sectype="

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.eastmoney.com")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("eastmoney search failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp struct {
		QuotationCodeTable struct {
			Data []struct {
				Code   string `json:"Code"`
				Name   string `json:"Name"`
				MktNum string `json:"MktNum"` // "0"=深圳, "1"=上海
			} `json:"Data"`
		} `json:"QuotationCodeTable"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	var out []AShareSearchResult
	for _, item := range apiResp.QuotationCodeTable.Data {
		if len(item.Code) != 6 {
			continue
		}
		market := "深交所"
		if item.MktNum == "1" {
			market = "上交所"
		}
		out = append(out, AShareSearchResult{
			Code:   item.Code,
			Name:   item.Name,
			Market: market,
		})
	}
	return out, nil
}
