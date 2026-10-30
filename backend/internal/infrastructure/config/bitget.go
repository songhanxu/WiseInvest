package config

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Environment credentials belong to one explicit WiseInvest user. Zero never
// grants access to a default/demo user or to all authenticated users.
type BitgetConfig struct {
	APIKey            string `json:"-"`
	APISecret         string `json:"-"`
	Passphrase        string `json:"-"`
	HoldingsAccessKey string `json:"-"`
	OwnerUserID       uint
	AccountType       string
	AccountName       string
	IncludeFutures    bool
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
	accessKey := strings.TrimSpace(getEnv("BITGET_HOLDINGS_ACCESS_KEY", ""))
	if accessKey != "" {
		if len(accessKey) != 64 {
			return BitgetConfig{}, fmt.Errorf("BITGET_HOLDINGS_ACCESS_KEY must be 64 hexadecimal characters (generate with openssl rand -hex 32)")
		}
		if _, err := hex.DecodeString(accessKey); err != nil {
			return BitgetConfig{}, fmt.Errorf("BITGET_HOLDINGS_ACCESS_KEY must be 64 hexadecimal characters (generate with openssl rand -hex 32)")
		}
		for _, other := range []string{getEnv("BITGET_API_KEY", ""), getEnv("BITGET_API_SECRET", ""), getEnv("BITGET_PASSPHRASE", "")} {
			if accessKey == other {
				return BitgetConfig{}, fmt.Errorf("BITGET_HOLDINGS_ACCESS_KEY must differ from Bitget credentials")
			}
		}
	}
	return BitgetConfig{APIKey: strings.TrimSpace(getEnv("BITGET_API_KEY", "")), APISecret: strings.TrimSpace(getEnv("BITGET_API_SECRET", "")), Passphrase: getEnv("BITGET_PASSPHRASE", ""), HoldingsAccessKey: accessKey, OwnerUserID: uint(owner), AccountType: accountType, AccountName: getEnv("BITGET_ACCOUNT_NAME", "Bitget"), IncludeFutures: futures}, nil
}

func (c BitgetConfig) HasCredentials() bool {
	return c.APIKey != "" && c.APISecret != "" && c.Passphrase != ""
}
