package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

func TestAutomationBitgetHoldingsRequiresSeparateKeyAndHTTPS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accessKey := strings.Repeat("a", 64)
	fixture := &accountFixture{}
	h := &AccountHandler{
		cfg:      config.BitgetConfig{APIKey: "bitget-key", APISecret: "bitget-secret", Passphrase: "bitget-pass", HoldingsAccessKey: accessKey, OwnerUserID: 2, AccountName: "Private Bitget", AccountType: "uta"},
		private:  fixture,
		snapshot: &HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{Quantity: "secret quantity"}}, Complete: true},
		cachedAt: time.Now(),
	}
	request := func(auth string, secure bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/automation/bitget/holdings", nil)
		if auth != "" {
			c.Request.Header.Set("Authorization", auth)
		}
		if secure {
			c.Request.TLS = &tls.ConnectionState{}
		}
		h.GetAutomationBitgetHoldings(c)
		return w
	}
	if w := request("Bearer "+accessKey, false); w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "secret quantity") {
		t.Fatalf("HTTP exposed holdings: %d %s", w.Code, w.Body.String())
	}
	for _, auth := range []string{"", "Bearer login-jwt", "Bearer " + strings.Repeat("b", 64), "Basic " + accessKey} {
		if w := request(auth, true); w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "secret quantity") {
			t.Fatalf("invalid credential exposed holdings: %d %s", w.Code, w.Body.String())
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("unauthorized request reached Bitget")
	}
	if w := request("Bearer "+accessKey, true); w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), "secret quantity") || strings.Contains(w.Body.String(), "bitget-secret") {
		t.Fatalf("authorized account response invalid: %d %s", w.Code, w.Body.String())
	}
	h.cfg.OwnerUserID = 0
	if w := request("Bearer "+accessKey, true); w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "secret quantity") {
		t.Fatalf("unbound account exposed: %d %s", w.Code, w.Body.String())
	}
	h.cfg.OwnerUserID = 2
	h.cfg.HoldingsAccessKey = ""
	if w := request("Bearer "+accessKey, true); w.Code != http.StatusNotFound {
		t.Fatalf("disabled endpoint available: %d %s", w.Code, w.Body.String())
	}
}

func TestAutomationHTTPSCannotBeSpoofedFromExternalAddress(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/automation/bitget/holdings", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.RemoteAddr = "198.51.100.1:4321"
	if secureAutomationRequest(r) {
		t.Fatal("external caller spoofed HTTPS")
	}
	r.RemoteAddr = "172.18.0.2:4321"
	if !secureAutomationRequest(r) {
		t.Fatal("trusted HTTPS proxy rejected")
	}
	r.Header.Set("X-Forwarded-Proto", "http")
	if secureAutomationRequest(r) {
		t.Fatal("plain HTTP proxy accepted")
	}
	r.Header.Del("X-Forwarded-Proto")
	if !secureAutomationRequest(r) {
		t.Fatal("direct Docker bridge request for SSH tunnel rejected")
	}
}
