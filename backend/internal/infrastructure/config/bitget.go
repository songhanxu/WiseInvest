package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Environment credentials belong to one explicit WiseInvest user. Zero never
// grants access to a default/demo user or to all authenticated users.
type BitgetConfig struct {
	APIKey         string `json:"-"`
	APISecret      string `json:"-"`
	Passphrase     string `json:"-"`
	OwnerUserID    uint
	AccountType    string
	AccountName    string
	IncludeFutures bool
}

func loadBitgetConfig() (BitgetConfig, error) {
	owner, err := strconv.ParseUint(getEnv("BITGET_OWNER_USER_ID", "0"), 10, 32)
	if err != nil {
		return BitgetConfig{}, fmt.Errorf("BITGET_OWNER_USER_ID must be a positive user ID, or 0 when unbound")
	}
	accountType := strings.ToLower(getEnv("BITGET_ACCOUNT_TYPE", "uta"))
	if accountType != "uta" && accountType != "classic" {
		return BitgetConfig{}, fmt.Errorf("BITGET_ACCOUNT_TYPE must be uta or classic")
	}
	futures, err := strconv.ParseBool(getEnv("BITGET_INCLUDE_FUTURES", "true"))
	if err != nil {
		return BitgetConfig{}, fmt.Errorf("BITGET_INCLUDE_FUTURES must be true or false")
	}
	return BitgetConfig{APIKey: strings.TrimSpace(getEnv("BITGET_API_KEY", "")), APISecret: strings.TrimSpace(getEnv("BITGET_API_SECRET", "")), Passphrase: getEnv("BITGET_PASSPHRASE", ""), OwnerUserID: uint(owner), AccountType: accountType, AccountName: getEnv("BITGET_ACCOUNT_NAME", "Bitget"), IncludeFutures: futures}, nil
}

func (c BitgetConfig) HasCredentials() bool {
	return c.APIKey != "" && c.APISecret != "" && c.Passphrase != ""
}
