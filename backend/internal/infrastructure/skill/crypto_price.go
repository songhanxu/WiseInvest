package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CryptoPriceSkill fetches cryptocurrency prices via Binance public API.
// No API key required. Uses /api/v3/ticker/24hr endpoint for 24h price statistics.
type CryptoPriceSkill struct{}

func NewCryptoPriceSkill() *CryptoPriceSkill { return &CryptoPriceSkill{} }

func (s *CryptoPriceSkill) Name() string { return "get_crypto_price" }

func (s *CryptoPriceSkill) Description() string {
	return "查询加密货币实时价格、24h涨跌幅、24h成交额等行情数据（数据来源：币安）。支持常用简称（BTC/ETH/SOL等）自动转换为币安交易对。"
}

func (s *CryptoPriceSkill) Parameters() []SkillParam {
	return []SkillParam{
		{
			Name:        "coins",
			Type:        "string",
			Description: "加密货币简称，多个用英文逗号分隔。例如：BTC, ETH, BNB, SOL, XRP, ADA, DOGE, AVAX, DOT, LINK, MATIC, LTC, ATOM, NEAR, TRX, BCH, SUI, APT, ARB, OP, TON, PEPE, SHIB。",
			Required:    true,
		},
	}
}

func (s *CryptoPriceSkill) Execute(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	coins, _ := input["coins"].(string)
	if coins == "" {
		return nil, fmt.Errorf("coins is required")
	}

	// Normalize input to Binance symbols: "btc,eth" → ["BTCUSDT","ETHUSDT"]
	var binanceSymbols []string
	var orderedBases []string // to maintain output order
	seen := make(map[string]bool)
	for _, p := range strings.Split(coins, ",") {
		p = strings.TrimSpace(strings.ToUpper(p))
		if p == "" {
			continue
		}
		// Handle various input formats
		base := p
		if strings.HasSuffix(p, "USDT") {
			base = strings.TrimSuffix(p, "USDT")
		} else if strings.HasSuffix(p, "/USDT") {
			base = strings.TrimSuffix(p, "/USDT")
		}
		// Also handle CoinGecko IDs passed by crypto_agent
		if mapped, ok := coinGeckoIDToSymbol[strings.ToLower(base)]; ok {
			base = mapped
		}
		sym := base + "USDT"
		if !seen[sym] {
			seen[sym] = true
			binanceSymbols = append(binanceSymbols, sym)
			orderedBases = append(orderedBases, base)
		}
	}
	if len(binanceSymbols) == 0 {
		return nil, fmt.Errorf("no valid coin symbols provided")
	}

	// Build Binance batch query: /api/v3/ticker/24hr?symbols=["BTCUSDT","ETHUSDT"]
	symbolsJSON := "[\"" + strings.Join(binanceSymbols, "\",\"") + "\"]"
	apiURL := fmt.Sprintf("https://data-api.binance.vision/api/v3/ticker/24hr?symbols=%s", url.QueryEscape(symbolsJSON))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch crypto data: %w", err)
	}
	defer resp.Body.Close()

	var tickers []struct {
		Symbol             string `json:"symbol"`
		PriceChange        string `json:"priceChange"`
		PriceChangePercent string `json:"priceChangePercent"`
		PrevClosePrice     string `json:"prevClosePrice"`
		LastPrice          string `json:"lastPrice"`
		OpenPrice          string `json:"openPrice"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		Volume             string `json:"volume"`
		QuoteVolume        string `json:"quoteVolume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(tickers) == 0 {
		return "未找到加密货币数据，请检查币种代码是否正确。", nil
	}

	// Index tickers by symbol for ordered output
	tickerMap := make(map[string]int, len(tickers))
	for i, t := range tickers {
		tickerMap[t.Symbol] = i
	}

	var sb strings.Builder
	for i, base := range orderedBases {
		sym := binanceSymbols[i]
		idx, ok := tickerMap[sym]
		if !ok {
			continue
		}
		t := tickers[idx]

		price := parseSkillFloat(t.LastPrice)
		changePct := parseSkillFloat(t.PriceChangePercent)
		change := parseSkillFloat(t.PriceChange)
		vol := parseSkillFloat(t.QuoteVolume) // USDT volume
		high := parseSkillFloat(t.HighPrice)
		low := parseSkillFloat(t.LowPrice)

		displayName := base
		if name, ok := cryptoDisplayNames[base]; ok {
			displayName = name
		}

		sb.WriteString(fmt.Sprintf("**%s（%s）**\n", displayName, base))
		sb.WriteString(fmt.Sprintf("  当前价：$%.6g USD\n", price))
		sb.WriteString(fmt.Sprintf("  24h 涨跌：%+.2f USD（%+.2f%%）\n", change, changePct))
		sb.WriteString(fmt.Sprintf("  24h 最高：$%.6g │ 最低：$%.6g\n", high, low))
		if vol > 0 {
			sb.WriteString(fmt.Sprintf("  24h 成交额：%.2f 亿 USD\n", vol/1e8))
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		return "未找到加密货币数据。", nil
	}
	return sb.String(), nil
}

// coinGeckoIDToSymbol maps CoinGecko IDs to Binance base symbols.
// This ensures backward compatibility when crypto_agent passes CoinGecko IDs.
var coinGeckoIDToSymbol = map[string]string{
	"bitcoin": "BTC", "ethereum": "ETH", "binancecoin": "BNB", "solana": "SOL",
	"ripple": "XRP", "cardano": "ADA", "dogecoin": "DOGE", "avalanche-2": "AVAX",
	"polkadot": "DOT", "chainlink": "LINK", "uniswap": "UNI", "matic-network": "MATIC",
	"litecoin": "LTC", "cosmos": "ATOM", "near": "NEAR", "fantom": "FTM",
	"tron": "TRX", "ethereum-classic": "ETC", "bitcoin-cash": "BCH", "sui": "SUI",
	"aptos": "APT", "arbitrum": "ARB", "optimism": "OP", "the-open-network": "TON",
	"pepe": "PEPE", "shiba-inu": "SHIB", "injective-protocol": "INJ", "sei-network": "SEI",
}

// cryptoDisplayNames maps uppercase base symbols to human-readable names.
var cryptoDisplayNames = map[string]string{
	"BTC": "Bitcoin", "ETH": "Ethereum", "BNB": "BNB", "SOL": "Solana",
	"XRP": "XRP", "ADA": "Cardano", "DOGE": "Dogecoin", "AVAX": "Avalanche",
	"DOT": "Polkadot", "LINK": "Chainlink", "UNI": "Uniswap", "MATIC": "Polygon",
	"LTC": "Litecoin", "ATOM": "Cosmos", "NEAR": "NEAR", "FTM": "Fantom",
	"TRX": "TRON", "ETC": "Ethereum Classic", "BCH": "Bitcoin Cash", "SUI": "Sui",
	"APT": "Aptos", "ARB": "Arbitrum", "OP": "Optimism", "TON": "Toncoin",
	"PEPE": "Pepe", "SHIB": "Shiba Inu", "INJ": "Injective", "SEI": "Sei",
}

// parseSkillFloat parses a numeric string to float64 (local helper to avoid dependency on handler package).
func parseSkillFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
