package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConnectedAccountBindingDefaultsAndSecretSerialization(t *testing.T) {
	t.Setenv("GATE_API_KEY", "private-key")
	t.Setenv("GATE_API_SECRET", "private-secret")
	t.Setenv("GATE_OWNER_USER_ID", "")
	c, err := loadExchangeAccount("GATE", "gate", "Gate")
	if err != nil || c.OwnerUserID != 0 || !c.HasCredentials() {
		t.Fatalf("unsafe defaults: %+v %v", c, err)
	}
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), "private-") {
		t.Fatal("credentials serialized")
	}
	t.Setenv("GATE_OWNER_USER_ID", "2")
	t.Setenv("GATE_ACCOUNT_TYPE", "unified")
	t.Setenv("GATE_INCLUDE_FUTURES", "false")
	c, err = loadExchangeAccount("GATE", "gate", "Gate")
	if err != nil || c.OwnerUserID != 2 || c.AccountType != "unified" || c.IncludeFutures {
		t.Fatal("configured account mode lost")
	}
	t.Setenv("GATE_OWNER_USER_ID", "not-a-user")
	if _, err = loadExchangeAccount("GATE", "gate", "Gate"); err == nil {
		t.Fatal("invalid owner accepted")
	}
	t.Setenv("GATE_OWNER_USER_ID", "0")
	t.Setenv("GATE_ACCOUNT_TYPE", "arbitrary")
	if _, err = loadExchangeAccount("GATE", "gate", "Gate"); err == nil {
		t.Fatal("unsupported account mode accepted")
	}
}
