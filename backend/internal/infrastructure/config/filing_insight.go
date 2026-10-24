package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type FilingInsightConfig struct {
	MaxTokens      int
	RetryMaxTokens int
	Timeout        time.Duration
	Thinking       string // auto: disable thinking only for supported DeepSeek models.
}

func DefaultFilingInsightConfig() FilingInsightConfig {
	return FilingInsightConfig{MaxTokens: 8192, RetryMaxTokens: 16384, Timeout: 180 * time.Second, Thinking: "auto"}
}

func loadFilingInsightConfig() (FilingInsightConfig, error) {
	cfg := DefaultFilingInsightConfig()
	for _, item := range []struct {
		name  string
		value *int
	}{
		{"FILING_INSIGHT_MAX_TOKENS", &cfg.MaxTokens},
		{"FILING_INSIGHT_RETRY_MAX_TOKENS", &cfg.RetryMaxTokens},
	} {
		value, err := strconv.Atoi(getEnv(item.name, strconv.Itoa(*item.value)))
		if err != nil || value < 1 || value > 65536 {
			return cfg, fmt.Errorf("%s must be an integer between 1 and 65536", item.name)
		}
		*item.value = value
	}
	if cfg.RetryMaxTokens < cfg.MaxTokens {
		return cfg, fmt.Errorf("FILING_INSIGHT_RETRY_MAX_TOKENS must be >= FILING_INSIGHT_MAX_TOKENS")
	}
	var err error
	cfg.Timeout, err = time.ParseDuration(getEnv("FILING_INSIGHT_TIMEOUT", cfg.Timeout.String()))
	if err != nil || cfg.Timeout < 30*time.Second || cfg.Timeout > 10*time.Minute {
		return cfg, fmt.Errorf("FILING_INSIGHT_TIMEOUT must be between 30s and 10m")
	}
	cfg.Thinking = strings.ToLower(strings.TrimSpace(getEnv("FILING_INSIGHT_THINKING", cfg.Thinking)))
	if cfg.Thinking != "auto" && cfg.Thinking != "enabled" && cfg.Thinking != "disabled" {
		return cfg, fmt.Errorf("FILING_INSIGHT_THINKING must be auto, enabled or disabled")
	}
	return cfg, nil
}
