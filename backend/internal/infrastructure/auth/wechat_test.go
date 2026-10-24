package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiniProgramCodeUsesCode2Session(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sns/jscode2session" || r.URL.Query().Get("appid") != "mini-app" ||
			r.URL.Query().Get("secret") != "mini-secret" || r.URL.Query().Get("js_code") != "wx-code" ||
			r.URL.Query().Get("grant_type") != "authorization_code" {
			t.Errorf("unexpected code exchange request: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"openid":"mini-open-id","unionid":"shared-union-id","session_key":"private-key"}`))
	}))
	defer server.Close()
	svc := NewWeChatService("ios-app", "ios-secret")
	svc.ConfigureMiniProgram("mini-app", "mini-secret")
	svc.apiBase = server.URL
	session, err := svc.ExchangeMiniProgramCode(context.Background(), "wx-code")
	if err != nil {
		t.Fatal(err)
	}
	if session.OpenID != "mini-open-id" || session.UnionID != "shared-union-id" {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestMiniProgramRejectsWeChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":40029,"errmsg":"invalid code"}`))
	}))
	defer server.Close()
	svc := NewWeChatService("", "")
	svc.ConfigureMiniProgram("mini-app", "mini-secret")
	svc.apiBase = server.URL
	if _, err := svc.ExchangeMiniProgramCode(context.Background(), "bad-code"); err == nil {
		t.Fatal("invalid code was accepted")
	}
}
