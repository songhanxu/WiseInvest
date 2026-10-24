package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type fakeCodeService struct{}

func (fakeCodeService) IsConfigured() bool                             { return true }
func (fakeCodeService) SendCode(context.Context, string, string) error { return nil }
func (fakeCodeService) VerifyCode(_ context.Context, _, code string) (bool, error) {
	return code == "123456", nil
}

func TestWebAccountRegistrationAndLogin(t *testing.T) {
	dsn := os.Getenv("WISEINVEST_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("set WISEINVEST_AUTH_TEST_DSN to an isolated PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	h := NewAuthHandler(db, auth.NewJWTService("test-secret", time.Hour), nil, fakeCodeService{}, fakeCodeService{}, logger.NewLogger())
	var bindingUserID, emailUserID uint
	call := func(path, body string) (int, map[string]any) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		switch path {
		case "/register":
			h.Register(c)
		case "/email/login":
			h.EmailLogin(c)
		case "/phone/login":
			h.PhoneLogin(c)
		case "/email/bind":
			c.Set("userID", bindingUserID)
			h.BindEmail(c)
		case "/phone/bind":
			c.Set("userID", emailUserID)
			h.BindPhone(c)
		default:
			h.Login(c)
		}
		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return w.Code, response
	}
	if code, _ := call("/register", `{"username":"pilot_01","email":"bad","code":"123456","password":"long-enough-password"}`); code != 400 {
		t.Fatalf("invalid email: %d", code)
	}
	if code, _ := call("/register", `{"username":"pilot_01","email":"investor@example.com","code":"123456","password":"short"}`); code != 400 {
		t.Fatalf("short password: %d", code)
	}
	if code, _ := call("/register", `{"username":"pilot_01","email":"investor@example.com","code":"000000","password":"long-enough-password"}`); code != 401 {
		t.Fatalf("wrong registration code: %d", code)
	}
	code, response := call("/register", `{"username":"Pilot_01","email":"Investor@Example.com","code":"123456","password":"long-enough-password"}`)
	if code != 200 || response["token"] == "" {
		t.Fatalf("registration failed: %d, %v", code, response)
	}
	emailUserID = uint(response["user"].(map[string]any)["id"].(float64))
	var user model.User
	if err := db.Where("email = ?", "investor@example.com").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Username != "pilot_01" || user.PasswordHash == "" || user.PasswordHash == "long-enough-password" {
		t.Fatal("password was not hashed")
	}
	if code, _ := call("/register", `{"username":"bad name","email":"different@example.com","code":"123456","password":"long-enough-password"}`); code != 400 {
		t.Fatalf("invalid username: %d", code)
	}
	if code, _ := call("/register", `{"username":"pilot_01","email":"different@example.com","code":"123456","password":"long-enough-password"}`); code != 409 {
		t.Fatalf("duplicate username: %d", code)
	}
	if code, _ := call("/register", `{"username":"pilot_02","email":"investor@example.com","code":"123456","password":"long-enough-password"}`); code != 409 {
		t.Fatalf("duplicate registration: %d", code)
	}
	if code, _ := call("/login", `{"email":"investor@example.com","password":"wrong-password"}`); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	if code, _ := call("/login", `{"email":"other@example.com","password":"long-enough-password"}`); code != 401 {
		t.Fatalf("unknown account: %d", code)
	}
	if code, response := call("/login", `{"email":"INVESTOR@example.com","password":"long-enough-password"}`); code != 200 || response["token"] == "" {
		t.Fatalf("login failed: %d, %v", code, response)
	}
	if code, response := call("/login", `{"username":"PILOT_01","password":"long-enough-password"}`); code != 200 || response["token"] == "" {
		t.Fatalf("username login failed: %d, %v", code, response)
	}
	if code, response := call("/email/login", `{"email":"INVESTOR@example.com","code":"123456"}`); code != 200 || response["token"] == "" {
		t.Fatalf("email code login failed: %d, %v", code, response)
	}
	if code, response := call("/phone/bind", `{"phone":"13900139000","code":"123456"}`); code != 200 || response["token"] == "" {
		t.Fatalf("phone binding failed: %d, %v", code, response)
	}
	if code, response := call("/login", `{"phone":"13900139000","password":"long-enough-password"}`); code != 200 || response["user"].(map[string]any)["id"] != float64(emailUserID) {
		t.Fatalf("bound phone should log into same user: %d, %v", code, response)
	}
	if code, response := call("/register", `{"username":"phone_user","phone":"13800138000","code":"123456","password":"long-enough-password"}`); code != 200 || response["token"] == "" {
		t.Fatalf("phone registration failed: %d, %v", code, response)
	} else {
		bindingUserID = uint(response["user"].(map[string]any)["id"].(float64))
	}
	if code, response := call("/login", `{"phone":"13800138000","password":"long-enough-password"}`); code != 200 || response["token"] == "" {
		t.Fatalf("phone password login failed: %d, %v", code, response)
	}
	if code, response := call("/phone/login", `{"phone":"13800138000","code":"123456"}`); code != 200 || response["token"] == "" {
		t.Fatalf("phone code login failed: %d, %v", code, response)
	}
	if code, _ := call("/phone/login", `{"phone":"13700137000","code":"123456","create":false}`); code != 401 {
		t.Fatalf("web code login must not create an unregistered account: %d", code)
	}
	if code, response := call("/email/bind", `{"email":"phone-owner@example.com","code":"123456"}`); code != 200 || response["token"] == "" {
		t.Fatalf("email binding failed: %d, %v", code, response)
	}
	if code, response := call("/login", `{"email":"phone-owner@example.com","password":"long-enough-password"}`); code != 200 || response["user"].(map[string]any)["id"] != float64(bindingUserID) {
		t.Fatalf("bound email should log into same user: %d, %v", code, response)
	}
	if code, response := call("/email/login", `{"email":"phone-owner@example.com","code":"123456"}`); code != 200 || response["user"].(map[string]any)["id"] != float64(bindingUserID) {
		t.Fatalf("bound email code should log into same user: %d, %v", code, response)
	}
}

func TestMockWechatLoginRequiresExplicitLocalOptIn(t *testing.T) {
	t.Setenv("SERVER_MODE", "release")
	t.Setenv("ALLOW_MOCK_AUTH", "false")
	gin.SetMode(gin.TestMode)
	h := NewAuthHandler(nil, nil, auth.NewWeChatService("", ""), nil, nil, logger.NewLogger())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/wechat/login", strings.NewReader(`{"code":"MOCK_WEB_LOGIN"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.WeChatLogin(c)
	if w.Code != 501 {
		t.Fatalf("mock login should be unavailable outside local development: %d", w.Code)
	}
}
