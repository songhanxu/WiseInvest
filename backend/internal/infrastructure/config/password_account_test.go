package config

import "testing"

func TestAccountBootstrapRequiresExplicitExistingUser(t *testing.T) {
	t.Setenv("AUTH_BOOTSTRAP_USER_ID", "")
	t.Setenv("AUTH_BOOTSTRAP_USERNAME", "")
	t.Setenv("AUTH_BOOTSTRAP_PASSWORD", "")
	cfg, err := Load()
	if err != nil || cfg.AccountBootstrap.UserID != 0 {
		t.Fatal("disabled bootstrap changed configuration")
	}
	t.Setenv("AUTH_BOOTSTRAP_USERNAME", "admin")
	if _, err := Load(); err == nil {
		t.Fatal("partial bootstrap configuration accepted")
	}
	t.Setenv("AUTH_BOOTSTRAP_USER_ID", "0")
	t.Setenv("AUTH_BOOTSTRAP_PASSWORD", "test-password")
	if _, err := Load(); err == nil {
		t.Fatal("zero user ID accepted")
	}
	t.Setenv("AUTH_BOOTSTRAP_USER_ID", "2")
	cfg, err = Load()
	if err != nil || cfg.AccountBootstrap.UserID != 2 || cfg.AccountBootstrap.Username != "admin" || cfg.AccountBootstrap.Password != "test-password" {
		t.Fatal("bootstrap configuration was not preserved")
	}
}
