package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBitgetConfigDisabledDefaultsAndValidation(t *testing.T) {
	for _, key := range []string{"BITGET_API_KEY", "BITGET_API_SECRET", "BITGET_PASSPHRASE", "BITGET_OWNER_USER_ID", "BITGET_ACCOUNT_TYPE", "BITGET_ACCOUNT_NAME", "BITGET_INCLUDE_FUTURES", "BITGET_HOLDINGS_ACCESS_KEY", "BITGET_HOLDINGS_SNAPSHOT_JSON", "BITGET_HOLDINGS_SNAPSHOT_FILE"} {
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
	for _, invalid := range []string{"short", strings.Repeat("z", 64), "private-key"} {
		t.Setenv("BITGET_HOLDINGS_ACCESS_KEY", invalid)
		if _, err := loadBitgetConfig(); err == nil {
			t.Fatalf("invalid automation key accepted: %q", invalid)
		}
	}
	accessKey := strings.Repeat("a", 64)
	t.Setenv("BITGET_HOLDINGS_ACCESS_KEY", accessKey)
	c, err = loadBitgetConfig()
	encoded, _ = json.Marshal(c)
	if err != nil || c.HoldingsAccessKey != accessKey || strings.Contains(string(encoded), accessKey) {
		t.Fatal("automation key config invalid or exposed")
	}
	snapshotJSON := `{"account_id":"bitget","holdings":[]}`
	t.Setenv("BITGET_HOLDINGS_SNAPSHOT_JSON", snapshotJSON)
	c, err = loadBitgetConfig()
	encoded, _ = json.Marshal(c)
	if err != nil || c.HoldingsSnapshotJSON != snapshotJSON || strings.Contains(string(encoded), snapshotJSON) {
		t.Fatal("inline holdings snapshot config invalid or exposed")
	}
	t.Setenv("BITGET_API_KEY", accessKey)
	if _, err := loadBitgetConfig(); err == nil {
		t.Fatal("Bitget API key reused as automation key")
	}
}
