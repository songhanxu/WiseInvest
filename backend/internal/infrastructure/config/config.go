package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Server           ServerConfig
	Database         DatabaseConfig
	Redis            RedisConfig
	OpenAI           OpenAIConfig
	FilingInsight    FilingInsightConfig
	Binance          BinanceConfig
	Bitget           BitgetConfig
	BinanceAccount   ExchangeAccountConfig
	GateAccount      ExchangeAccountConfig
	Search           SearchConfig
	JWT              JWTConfig
	AccountBootstrap AccountBootstrapConfig
	CORS             CORSConfig
	WeChat           WeChatConfig
	Notification     NotificationConfig
}

// NotificationConfig holds push notification configuration
type NotificationConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string `json:"-"`
	SMTPFrom     string
	SMTPTLSMode  string
	// 企业微信机器人 Webhook URL
	WxWorkWebhookURL string
	// APNs (.p8 token-based auth)
	APNSKeyID      string
	APNSTeamID     string
	APNSBundleID   string
	APNSKeyFile    string // path to .p8 file
	APNSProduction bool
}

// WeChatConfig holds WeChat OAuth configuration
type WeChatConfig struct {
	AppID         string
	AppSecret     string
	MiniAppID     string
	MiniAppSecret string
}

// SearchConfig holds web search configuration.
// SEARCH_PROVIDER (serper|brave) and SEARCH_API_KEY configure a fallback when
// the official DeepSeek API is used, or the primary provider otherwise.
type SearchConfig struct {
	Provider string // "serper" | "brave" | "" (disabled)
	APIKey   string
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port string
	Mode string
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// OpenAIConfig holds OpenAI configuration
type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret     string
	Expiration time.Duration
}

// Optional one-time migration of an explicitly selected existing account.
type AccountBootstrapConfig struct {
	UserID   uint
	Username string
	Password string `json:"-"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins []string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	bootstrap := AccountBootstrapConfig{Username: os.Getenv("AUTH_BOOTSTRAP_USERNAME"), Password: os.Getenv("AUTH_BOOTSTRAP_PASSWORD")}
	if rawID := os.Getenv("AUTH_BOOTSTRAP_USER_ID"); rawID != "" || bootstrap.Username != "" || bootstrap.Password != "" {
		id, err := strconv.ParseUint(rawID, 10, 32)
		if err != nil || id == 0 || bootstrap.Username == "" || bootstrap.Password == "" {
			return nil, fmt.Errorf("AUTH_BOOTSTRAP_USER_ID, AUTH_BOOTSTRAP_USERNAME and AUTH_BOOTSTRAP_PASSWORD must be configured together")
		}
		bootstrap.UserID = uint(id)
	}
	bitgetConfig, err := loadBitgetConfig()
	if err != nil {
		return nil, err
	}
	binanceAccount, err := loadExchangeAccount("BINANCE", "binance", "币安")
	if err != nil {
		return nil, err
	}
	gateAccount, err := loadExchangeAccount("GATE", "gate", "Gate")
	if err != nil {
		return nil, err
	}
	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	jwtExpiration, err := time.ParseDuration(getEnv("JWT_EXPIRATION", "24h"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRATION: %w", err)
	}
	filingInsight, err := loadFilingInsightConfig()
	if err != nil {
		return nil, err
	}

	return &Config{
		FilingInsight:    filingInsight,
		AccountBootstrap: bootstrap,
		Bitget:           bitgetConfig,
		BinanceAccount:   binanceAccount, GateAccount: gateAccount,
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Mode: getEnv("SERVER_MODE", "debug"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "wiseinvest"),
			Password: getEnv("DB_PASSWORD", "wiseinvest"),
			DBName:   getEnv("DB_NAME", "wiseinvest"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		OpenAI: OpenAIConfig{
			APIKey:  getEnv("OPENAI_API_KEY", ""),
			Model:   getEnv("OPENAI_MODEL", "gpt-4-turbo-preview"),
			BaseURL: getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		},
		Search: SearchConfig{
			Provider: getEnv("SEARCH_PROVIDER", ""),
			APIKey:   getEnv("SEARCH_API_KEY", ""),
		},
		Binance: BinanceConfig{
			APIKey:    getEnv("BINANCE_API_KEY", ""),
			APISecret: getEnv("BINANCE_API_SECRET", ""),
			BaseURL:   getEnv("BINANCE_BASE_URL", "https://data-api.binance.vision"),
			Testnet:   getEnv("BINANCE_TESTNET", "false") == "true",
		},
		JWT: JWTConfig{
			Secret:     getEnv("JWT_SECRET", "change-me-in-production"),
			Expiration: jwtExpiration,
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{
				getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
			},
		},
		WeChat: WeChatConfig{
			AppID:         getEnv("WECHAT_APP_ID", ""),
			AppSecret:     getEnv("WECHAT_APP_SECRET", ""),
			MiniAppID:     getEnv("WECHAT_MINI_APP_ID", ""),
			MiniAppSecret: getEnv("WECHAT_MINI_APP_SECRET", ""),
		},
		Notification: NotificationConfig{
			SMTPHost: getEnv("SMTP_HOST", ""), SMTPPort: getEnv("SMTP_PORT", "587"), SMTPUsername: getEnv("SMTP_USERNAME", ""), SMTPPassword: getEnv("SMTP_PASSWORD", ""), SMTPFrom: getEnv("SMTP_FROM", ""), SMTPTLSMode: getEnv("SMTP_TLS_MODE", "starttls"),
			WxWorkWebhookURL: getEnv("WXWORK_WEBHOOK_URL", ""),
			APNSKeyID:        getEnv("APNS_KEY_ID", ""),
			APNSTeamID:       getEnv("APNS_TEAM_ID", ""),
			APNSBundleID:     getEnv("APNS_BUNDLE_ID", ""),
			APNSKeyFile:      getEnv("APNS_KEY_FILE", "apns_key.p8"),
			APNSProduction:   getEnv("APNS_PRODUCTION", "false") == "true",
		},
	}, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// DSN returns the database connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// Address returns the Redis address
func (c *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}
