package config

// BinanceConfig represents Binance API configuration
type BinanceConfig struct {
	APIKey    string
	APISecret string
	BaseURL   string
	Testnet   bool
}

// LoadBinanceConfig loads Binance configuration from environment
func LoadBinanceConfig() *BinanceConfig {
	return &BinanceConfig{
		APIKey:    getEnv("BINANCE_API_KEY", ""),
		APISecret: getEnv("BINANCE_API_SECRET", ""),
		BaseURL:   getEnv("BINANCE_BASE_URL", "https://api.binance.com"),
		Testnet:   getEnv("BINANCE_TESTNET", "false") == "true",
	}
}
