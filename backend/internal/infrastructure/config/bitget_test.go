package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBitgetConfigDisabledDefaultsAndValidation(t *testing.T) {
	for _, key := range []string{"BITGET_API_KEY", "BITGET_API_SECRET", "BITGET_PASSPHRASE", "BITGET_OWNER_USER_ID", "BITGET_ACCOUNT_TYPE", "BITGET_ACCOUNT_NAME", "BITGET_INCLUDE_FUTURES"} {
		t.Setenv(key, "")
	}
	c, err := loadBitgetConfig()
	if err != nil || c.HasCredentials() || c.OwnerUserID != 0 || c.AccountType != "uta" || !c.IncludeFutures {
		t.Fatalf("unsafe defaults %+v %v", c, err)
	}
	t.Setenv("BITGET_API_KEY", "private-key")
	t.Setenv("BITGET_API_SECRET", "private-secret")
	t.Setenv("BITGET_PASSPHRASE", "private-pass")
	c, err = loadBitgetConfig()
	encoded, _ := json.Marshal(c)
	if err != nil || !c.HasCredentials() || strings.Contains(string(encoded), "private-") {
		t.Fatal("credentials exposed in config")
	}
	for _, key := range []string{"BITGET_OWNER_USER_ID", "BITGET_ACCOUNT_TYPE", "BITGET_INCLUDE_FUTURES"} {
		t.Setenv(key, "invalid")
		if _, err = loadBitgetConfig(); err == nil {
			t.Fatalf("invalid %s accepted", key)
		}
		t.Setenv(key, "")
	}
}
