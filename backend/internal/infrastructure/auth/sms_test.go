package auth

import (
	"context"
	"errors"
	"testing"
)

func TestSMSDoesNotPretendToSendWithoutProvider(t *testing.T) {
	t.Setenv("SERVER_MODE", "release")
	t.Setenv("ALLOW_MOCK_AUTH", "false")
	for _, key := range []string{
		"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY", "TENCENT_SMS_SDK_APP_ID",
		"TENCENT_SMS_SIGN_NAME", "TENCENT_SMS_TEMPLATE_ID",
	} {
		t.Setenv(key, "")
	}
	svc := NewSMSService(nil)
	if err := svc.SendCode(context.Background(), "13800138000", "127.0.0.1"); !errors.Is(err, ErrSMSUnavailable) {
		t.Fatalf("expected unavailable provider, got %v", err)
	}
}
