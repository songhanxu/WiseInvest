package config

import (
	"strings"
	"testing"
	"time"
)

func clearFilingEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"OPENAI_MODEL_PRO", "FILING_INSIGHT_MAX_TOKENS", "FILING_INSIGHT_RETRY_MAX_TOKENS", "FILING_INSIGHT_TIMEOUT", "FILING_INSIGHT_THINKING"} {
		t.Setenv(key, "")
	}
}

func TestProFilingDefaultsAndExplicitBudget(t *testing.T) {
	clearFilingEnvironment(t)
	t.Setenv("OPENAI_MODEL_PRO", "deepseek-v4-pro")
	cfg, err := Load()
	if err != nil || cfg.OpenAI.ModelPro != "deepseek-v4-pro" || cfg.FilingInsight.MaxTokens != 32768 || cfg.FilingInsight.RetryMaxTokens != 65536 || cfg.FilingInsight.Timeout != 10*time.Minute {
		t.Fatalf("wrong Pro defaults: %+v %v", cfg, err)
	}
	t.Setenv("FILING_INSIGHT_MAX_TOKENS", "4096")
	t.Setenv("FILING_INSIGHT_RETRY_MAX_TOKENS", "8192")
	t.Setenv("FILING_INSIGHT_TIMEOUT", "4m")
	cfg, err = Load()
	if err != nil || cfg.FilingInsight.MaxTokens != 4096 || cfg.FilingInsight.RetryMaxTokens != 8192 || cfg.FilingInsight.Timeout != 4*time.Minute {
		t.Fatalf("explicit Pro budgets ignored: %+v %v", cfg, err)
	}
}

func TestFilingInsightDefaultsAndOverrides(t *testing.T) {
	clearFilingEnvironment(t)
	cfg, err := Load()
	if err != nil || cfg.FilingInsight != DefaultFilingInsightConfig() {
		t.Fatalf("wrong default configuration: %+v %v", cfg, err)
	}
	t.Setenv("FILING_INSIGHT_MAX_TOKENS", "4096")
	t.Setenv("FILING_INSIGHT_RETRY_MAX_TOKENS", "8192")
	t.Setenv("FILING_INSIGHT_TIMEOUT", "4m")
	t.Setenv("FILING_INSIGHT_THINKING", "enabled")
	cfg, err = Load()
	if err != nil || cfg.FilingInsight != (FilingInsightConfig{4096, 8192, 4 * time.Minute, "enabled"}) {
		t.Fatalf("wrong custom configuration: %+v %v", cfg, err)
	}
}

func TestFilingInsightRejectsInvalidBudgetsAtStartup(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"FILING_INSIGHT_MAX_TOKENS", "0"}, {"FILING_INSIGHT_MAX_TOKENS", "-1"},
		{"FILING_INSIGHT_MAX_TOKENS", "text"}, {"FILING_INSIGHT_MAX_TOKENS", "65537"},
		{"FILING_INSIGHT_RETRY_MAX_TOKENS", "4096"}, {"FILING_INSIGHT_RETRY_MAX_TOKENS", "-1"},
		{"FILING_INSIGHT_TIMEOUT", "2s"}, {"FILING_INSIGHT_TIMEOUT", "1h"},
		{"FILING_INSIGHT_TIMEOUT", "invalid"}, {"FILING_INSIGHT_THINKING", "yes"},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			clearFilingEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("bad setting was not explained: %v", err)
			}
		})
	}
}
