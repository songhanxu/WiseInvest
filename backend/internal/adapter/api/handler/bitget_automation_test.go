package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
)

func TestAutomationBitgetHoldingsRequiresSeparateKeyAndLimitsReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accessKey := strings.Repeat("a", 64)
	fixture := &accountFixture{}
	h := &AccountHandler{
		cfg:      config.BitgetConfig{APIKey: "bitget-key", APISecret: "bitget-secret", Passphrase: "bitget-pass", HoldingsAccessKey: accessKey, OwnerUserID: 2, AccountName: "Private Bitget", AccountType: "uta"},
		private:  fixture,
		snapshot: &HoldingsSnapshot{AccountID: "bitget", Holdings: []AccountHolding{{Quantity: "secret quantity"}}, Complete: true},
		cachedAt: time.Now(),
	}
	request := func(auth string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/automation/bitget/holdings", nil)
		if auth != "" {
			c.Request.Header.Set("Authorization", auth)
		}
		h.GetAutomationBitgetHoldings(c)
		return w
	}
	for _, auth := range []string{"", "Bearer login-jwt", "Bearer " + strings.Repeat("b", 64), "Basic " + accessKey} {
		if w := request(auth); w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "secret quantity") {
			t.Fatalf("invalid credential exposed holdings: %d %s", w.Code, w.Body.String())
		}
	}
	if fixture.calls.Load() != 0 {
		t.Fatal("unauthorized request reached Bitget")
	}
	if w := request("Bearer " + accessKey); w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), "secret quantity") || strings.Contains(w.Body.String(), "bitget-secret") {
		t.Fatalf("authorized account response invalid: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < 5; i++ {
		if w := request("Bearer " + accessKey); w.Code != http.StatusOK {
			t.Fatalf("read %d rejected early: %d %s", i+2, w.Code, w.Body.String())
		}
	}
	if w := request("Bearer " + accessKey); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" || strings.Contains(w.Body.String(), "secret quantity") {
		t.Fatalf("read limit bypassed: %d %s", w.Code, w.Body.String())
	}
	h.cfg.OwnerUserID = 0
	if w := request("Bearer " + accessKey); w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "secret quantity") {
		t.Fatalf("unbound account exposed: %d %s", w.Code, w.Body.String())
	}
	h.cfg.OwnerUserID = 2
	h.cfg.HoldingsAccessKey = ""
	if w := request("Bearer " + accessKey); w.Code != http.StatusNotFound {
		t.Fatalf("disabled endpoint available: %d %s", w.Code, w.Body.String())
	}
}
