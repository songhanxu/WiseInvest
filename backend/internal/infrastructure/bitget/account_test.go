package bitget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrivateSignatureReadOnlyAndSanitizedErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.RawQuery != "category=USDT-FUTURES&symbol=TSLAUSDT" {
			t.Errorf("unexpected signed request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("required JSON Content-Type missing")
		}
		if r.Header.Get("ACCESS-KEY") != "test-key" || r.Header.Get("ACCESS-PASSPHRASE") != "test-pass" || r.Header.Get("ACCESS-TIMESTAMP") != "1700000000000" || r.Header.Get("ACCESS-SIGN") != "JwxDh1cVXAS+bZVM0d1MdFNnbdnGL+a0pDQuzkue47Y=" {
			t.Error("incorrect authentication headers")
		}
		w.Write([]byte(`{"code":"test-key","msg":"test-secret test-pass","data":null}`))
	}))
	defer server.Close()
	c := NewAccountClient(Credentials{APIKey: "test-key", Secret: "test-secret", Passphrase: "test-pass"})
	c.BaseURL = server.URL
	c.Now = func() time.Time { return time.UnixMilli(1700000000000) }
	var data any
	err := c.PrivateGET(context.Background(), "/api/v3/position/current-position", url.Values{"symbol": {"TSLAUSDT"}, "category": {"USDT-FUTURES"}}, &data)
	if err == nil || strings.Contains(err.Error(), "test-") {
		t.Fatalf("unsafe error: %v", err)
	}
	if err = c.PrivateGET(context.Background(), "/api/v3/trade/place-order", nil, &data); err == nil || calls.Load() != 1 {
		t.Fatal("non-read endpoint was requested")
	}
	encoded, _ := json.Marshal(c.credentials)
	if string(encoded) != "{}" {
		t.Fatal("credentials serialized")
	}
}

func TestAccountErrorsKeepSafeCodeEvenOnHTTPFailure(t *testing.T) {
	for _, code := range []string{"40007", "40014", "40036", "40038", "40008", "40009", "99999", "unsafe-secret"} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "private-key private-secret private-pass private-account", "data": nil})
			}))
			defer server.Close()
			c := NewAccountClient(Credentials{APIKey: "private-key", Secret: "private-secret", Passphrase: "private-pass"})
			c.BaseURL = server.URL
			_, err := c.Assets(context.Background(), "uta")
			if err == nil {
				t.Fatal("provider rejection accepted")
			}
			message := AccountErrorDescription(err)
			if strings.Contains(message, "private-") || strings.Contains(message, "unsafe-secret") || !strings.Contains(message, "HTTP 400") {
				t.Fatalf("unsafe or missing HTTP diagnostic: %s", message)
			}
			if code != "unsafe-secret" && !strings.Contains(message, code) {
				t.Fatalf("provider code lost: %s", message)
			}
		})
	}
	if message := AccountErrorDescription(accountConnectionError(context.DeadlineExceeded)); !strings.Contains(message, "超时") {
		t.Fatal("timeout not identified")
	}
}

func TestPrivateRedirectDoesNotForwardCredentials(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer origin.Close()
	c := NewAccountClient(Credentials{APIKey: "key", Secret: "secret", Passphrase: "pass"})
	c.BaseURL = origin.URL
	var data any
	if c.PrivateGET(context.Background(), "/api/v3/account/assets", nil, &data) == nil || calls.Load() != 0 {
		t.Fatal("redirect leaked authentication")
	}
}

func TestAccountDecodingAndDecimalHoldings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/account/assets":
			w.Write([]byte(`{"code":"00000","data":{"assets":[{"coin":"rAAPL","balance":"0.000000000000000001","equity":"9"}]}}`))
		case "/api/v3/position/current-position":
			if r.URL.Query().Get("category") != "USDT-FUTURES" {
				t.Error("wrong category")
			}
			w.Write([]byte(`{"code":"00000","data":{"list":[{"symbol":"TSLAUSDT","total":"3.2","posSide":"short"}]}}`))
		case "/api/v2/spot/account/assets":
			w.Write([]byte(`{"code":"00000","data":[{"coin":"rAAPL","available":"0.1","frozen":"0.2","locked":"0.00000001"}]}`))
		case "/api/v2/mix/position/all-position":
			if r.URL.Query().Get("productType") != "USDT-FUTURES" {
				t.Error("wrong product type")
			}
			w.Write([]byte(`{"code":"00000","data":[{"symbol":"TSLAUSDT","total":"2","holdSide":"long"}]}`))
		}
	}))
	defer server.Close()
	c := NewAccountClient(Credentials{APIKey: "key", Secret: "secret", Passphrase: "pass"})
	c.BaseURL = server.URL
	for _, accountType := range []string{"uta", "classic"} {
		assets, err := c.Assets(context.Background(), accountType)
		if err != nil || len(assets) != 1 {
			t.Fatalf("assets: %+v %v", assets, err)
		}
		want := "0.000000000000000001"
		if accountType == "classic" {
			want = "0.30000001"
		}
		if q := AssetQuantity(assets[0], accountType); q != want || !PositiveQuantity(q) {
			t.Fatalf("quantity %s, want %s", q, want)
		}
		positions, err := c.Positions(context.Background(), accountType)
		if err != nil || len(positions) != 1 || positions[0].Side == "" {
			t.Fatalf("positions: %+v %v", positions, err)
		}
	}
	if PositiveQuantity("0") || PositiveQuantity("-1") || PositiveQuantity("NaN") {
		t.Fatal("invalid quantity is positive")
	}
}

func TestMissingAccountListIsNotAnEmptyPortfolio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"code":"00000","data":{}}`)) }))
	defer server.Close()
	c := NewAccountClient(Credentials{APIKey: "key", Secret: "secret", Passphrase: "pass"})
	c.BaseURL = server.URL
	if _, err := c.Assets(context.Background(), "uta"); err == nil {
		t.Fatal("missing assets accepted")
	}
	if _, err := c.Positions(context.Background(), "uta"); err == nil {
		t.Fatal("missing positions accepted")
	}
}
