package auth

import (
	"context"
	"errors"
	"testing"
)

func TestEmailCodeUnavailableWithoutProvider(t *testing.T) {
	t.Setenv("SERVER_MODE", "release")
	t.Setenv("ALLOW_MOCK_AUTH", "false")
	for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM"} {
		t.Setenv(key, "")
	}
	svc := NewEmailCodeService(nil)
	if svc.IsConfigured() {
		t.Fatal("email service must not report configured without SMTP")
	}
	if err := svc.SendCode(context.Background(), "user@example.com", "127.0.0.1"); !errors.Is(err, ErrEmailUnavailable) {
		t.Fatalf("expected unavailable provider, got %v", err)
	}
}
