package handler

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
)

var dockerProxyNetwork = netip.MustParsePrefix("172.16.0.0/12")

// GetAutomationBitgetHoldings exposes only the configured Bitget account to a
// separate read-only machine credential. It does not accept a login JWT.
func (h *AccountHandler) GetAutomationBitgetHoldings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.cfg.HoldingsAccessKey == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if !secureAutomationRequest(c.Request) {
		c.JSON(http.StatusForbidden, gin.H{"error": "外部访问须使用 HTTPS"})
		return
	}
	if !h.validHoldingsAccessKey(c.Request.Header.Values("Authorization")) {
		c.Header("WWW-Authenticate", `Bearer realm="bitget-holdings"`)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "访问密钥无效"})
		return
	}
	if !h.cfg.HasCredentials() || h.cfg.OwnerUserID == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bitget 账户尚未绑定"})
		return
	}
	data, err := h.holdings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, decorateSnapshot(data, h.summary(h.cfg.OwnerUserID)))
}

func (h *AccountHandler) validHoldingsAccessKey(headers []string) bool {
	if len(headers) != 1 || len(headers[0]) != len("Bearer ")+64 || !strings.EqualFold(headers[0][:len("Bearer ")], "Bearer ") {
		return false
	}
	provided, err := hex.DecodeString(headers[0][len("Bearer "):])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(h.cfg.HoldingsAccessKey)
	return err == nil && subtle.ConstantTimeCompare(provided, expected) == 1
}

func secureAutomationRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil || (!remote.Addr().IsLoopback() && !dockerProxyNetwork.Contains(remote.Addr())) {
		return false
	}
	// Nginx overwrites this header with its actual incoming scheme. A direct
	// loopback/Docker-bridge request with no proxy header is also allowed for
	// SSH port forwarding; public HTTP through Nginx always sends "http".
	proto := r.Header.Get("X-Forwarded-Proto")
	return proto == "https" || proto == ""
}
