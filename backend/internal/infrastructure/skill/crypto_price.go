package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CryptoPriceSkill fetches cryptocurrency prices via CoinGecko's free public API.
// No API key required. Supports the /simple/price endpoint with 24h change and volume.
type CryptoPriceSkill struct{}

func NewCryptoPriceSkill() *CryptoPriceSkill { return &CryptoPriceSkill{} }

func (s *CryptoPriceSkill) Name() string { return "get_crypto_price" }

func (s *CryptoPriceSkill) Description() string {
	return "查询加密货币实时价格、24h涨跌幅、24h成交额、市值等行情数据。支持常用简称（BTC/ETH/SOL等）自动转换为 CoinGecko ID。"
}

func (s *CryptoPriceSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "coins",
			Type:        "string",
			Description: "加密货币 ID 或常用简称，多个用英文逗号分隔。常用简称：BTC, ETH, BNB, SOL, XRP, ADA, DOGE, AVAX, DOT, LINK, MATIC, LTC, ATOM, NEAR, TRX, BCH。也可直接使用 CoinGecko ID（如 bitcoin, ethereum）。",
			Required:    true,
		},
		{
			Name:        "vs_currency",
			Type:        "string",
			Description: "计价货币，默认 usd。可选：usd, cny",
			Required:    false,
			Enum:        []string{"usd", "cny"},
		},
	}
}

func (s *CryptoPriceSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	coins, _ := input["coins"].(string)
	if coins == "" {
		return nil, fmt.Errorf("coins is required")
	}

	vsCurrency := "usd"
	if vc, ok := input["vs_currency"].(string); ok && vc != "" {
		vsCurrency = strings.ToLower(vc)
	}

	coinIDs := normalizeCoinIDs(coins)

	url := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=%s&include_24hr_change=true&include_24hr_vol=true&include_market_cap=true",
		strings.Join(coinIDs, ","),
		vsCurrency,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch crypto data: %w", err)
	}
	defer resp.Body.Close()

	// Response: { "bitcoin": { "usd": 65000, "usd_24h_change": 2.5, ... }, ... }
	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(data) == 0 {
		return "未找到加密货币数据，请检查币种 ID 是否正确。CoinGecko 免费 API 有频率限制，请稍候重试。", nil
	}

	// Maintain input order for readability
	var sb strings.Builder
	for _, coinID := range coinIDs {
		prices, ok := data[coinID]
		if !ok {
			continue
		}
		price := prices[vsCurrency]
		change24h := prices[vsCurrency+"_24h_change"]
		vol24h := prices[vsCurrency+"_24h_vol"]
		marketCap := prices[vsCurrency+"_market_cap"]

		displayName := coinIDToSymbol(coinID)
		currencyLabel := strings.ToUpper(vsCurrency)

		sb.WriteString(fmt.Sprintf("**%s**（%s）\n", displayName, coinID))
		if vsCurrency == "usd" {
			sb.WriteString(fmt.Sprintf("  当前价：$%.6g USD\n", price))
		} else {
			sb.WriteString(fmt.Sprintf("  当前价：%.6g %s\n", price, currencyLabel))
		}
		sb.WriteString(fmt.Sprintf("  24h 涨跌幅：%+.2f%%\n", change24h))
		if vol24h > 0 {
			sb.WriteString(fmt.Sprintf("  24h 成交额：%.2f 亿 %s\n", vol24h/1e8, currencyLabel))
		}
		if marketCap > 0 {
			sb.WriteString(fmt.Sprintf("  市值：%.2f 亿 %s\n", marketCap/1e8, currencyLabel))
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		return "未找到加密货币数据。", nil
	}
	return sb.String(), nil
}

// symbolToID maps common uppercase tickers to CoinGecko IDs.
var symbolToID = map[string]string{
	"btc":   "bitcoin",
	"eth":   "ethereum",
	"bnb":   "binancecoin",
	"sol":   "solana",
	"xrp":   "ripple",
	"ada":   "cardano",
	"doge":  "dogecoin",
	"avax":  "avalanche-2",
	"dot":   "polkadot",
	"link":  "chainlink",
	"uni":   "uniswap",
	"matic": "matic-network",
	"pol":   "matic-network",
	"ltc":   "litecoin",
	"atom":  "cosmos",
	"near":  "near",
	"ftm":   "fantom",
	"trx":   "tron",
	"etc":   "ethereum-classic",
	"bch":   "bitcoin-cash",
	"sui":   "sui",
	"apt":   "aptos",
	"arb":   "arbitrum",
	"op":    "optimism",
	"inj":   "injective-protocol",
	"sei":   "sei-network",
	"ton":   "the-open-network",
	"pepe":  "pepe",
	"shib":  "shiba-inu",
}

// normalizeCoinIDs converts ticker symbols and mixed input to CoinGecko IDs.
func normalizeCoinIDs(coins string) []string {
	parts := strings.Split(coins, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		if id, ok := symbolToID[p]; ok {
			p = id
		}
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}

// coinIDToSymbol returns a display symbol for a CoinGecko ID.
func coinIDToSymbol(id string) string {
	idToSymbol := map[string]string{
		"bitcoin":          "BTC",
		"ethereum":         "ETH",
		"binancecoin":      "BNB",
		"solana":           "SOL",
		"ripple":           "XRP",
		"cardano":          "ADA",
		"dogecoin":         "DOGE",
		"avalanche-2":      "AVAX",
		"polkadot":         "DOT",
		"chainlink":        "LINK",
		"uniswap":          "UNI",
		"matic-network":    "MATIC",
		"litecoin":         "LTC",
		"cosmos":           "ATOM",
		"near":             "NEAR",
		"fantom":           "FTM",
		"tron":             "TRX",
		"ethereum-classic": "ETC",
		"bitcoin-cash":     "BCH",
		"sui":              "SUI",
		"aptos":            "APT",
		"arbitrum":         "ARB",
		"optimism":         "OP",
		"the-open-network": "TON",
		"pepe":             "PEPE",
		"shiba-inu":        "SHIB",
	}
	if sym, ok := idToSymbol[id]; ok {
		return sym
	}
	return strings.ToUpper(id)
}
