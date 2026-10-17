package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Separate account binding from the pre-existing public market configuration.
type ExchangeAccountConfig struct {
	Provider       string
	APIKey         string `json:"-"`
	APISecret      string `json:"-"`
	OwnerUserID    uint
	AccountName    string
	AccountType    string
	IncludeFutures bool
	IncludeStocks  bool
}

func (c ExchangeAccountConfig) HasCredentials() bool { return c.APIKey != "" && c.APISecret != "" }
func loadExchangeAccount(prefix, provider, name string) (ExchangeAccountConfig, error) {
	c := ExchangeAccountConfig{Provider: provider, APIKey: strings.TrimSpace(getEnv(prefix+"_API_KEY", "")), APISecret: strings.TrimSpace(getEnv(prefix+"_API_SECRET", "")), AccountName: getEnv(prefix+"_ACCOUNT_NAME", name), AccountType: getEnv(prefix+"_ACCOUNT_TYPE", "spot")}
	owner, e := strconv.ParseUint(getEnv(prefix+"_OWNER_USER_ID", "0"), 10, 32)
	if e != nil {
		return c, fmt.Errorf("%s_OWNER_USER_ID must be a non-negative user ID", prefix)
	}
	c.OwnerUserID = uint(owner)
	if c.AccountType != "spot" && !(provider == "gate" && c.AccountType == "unified") {
		return c, fmt.Errorf("invalid %s_ACCOUNT_TYPE", prefix)
	}
	c.IncludeFutures, e = strconv.ParseBool(getEnv(prefix+"_INCLUDE_FUTURES", "true"))
	if e != nil {
		return c, fmt.Errorf("invalid %s_INCLUDE_FUTURES", prefix)
	}
	c.IncludeStocks, e = strconv.ParseBool(getEnv(prefix+"_INCLUDE_STOCKS", "true"))
	if e != nil {
		return c, fmt.Errorf("invalid %s_INCLUDE_STOCKS", prefix)
	}
	return c, nil
}
