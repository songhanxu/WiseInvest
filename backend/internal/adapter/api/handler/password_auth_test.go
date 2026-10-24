package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/api/middleware"
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/database"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	ormlog "gorm.io/gorm/logger"
)

func passwordTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WISEINVEST_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set WISEINVEST_TEST_POSTGRES_DSN for isolated authentication integration tests")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: ormlog.Default.LogMode(ormlog.Silent)})
	if err != nil {
		t.Fatal("test database unavailable")
	}
	schema := fmt.Sprintf("auth_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: ormlog.Default.LogMode(ormlog.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		sqlAdmin, _ := admin.DB()
		sqlAdmin.Close()
	})
	if err := db.AutoMigrate(&model.User{}, &model.Conversation{}, &model.Message{}, &model.WatchlistItem{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPasswordRegistrationLoginAndLegacyAccountMigration(t *testing.T) {
	db := passwordTestDB(t)
	legacy := model.User{ID: 2, Username: "wx_legacy", Email: "legacy@example.invalid", DisplayName: "Existing User", Preferences: model.JSONB{}}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	db.Exec("SELECT setval('users_id_seq', 2)")
	item := model.WatchlistItem{UserID: 2, Market: "us_stock", StockCode: "NVDA", Symbol: "NVDA", Name: "NVIDIA"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	bootstrap := config.AccountBootstrapConfig{UserID: 2, Username: "admin", Password: "migration-password"}
	if err := database.BootstrapPasswordAccount(db, bootstrap); err != nil {
		t.Fatal(err)
	}
	bootstrap.Password = "must-not-reset-password"
	if err := database.BootstrapPasswordAccount(db, bootstrap); err != nil {
		t.Fatal(err)
	}
	var migrated model.User
	db.First(&migrated, 2)
	if migrated.ID != legacy.ID || migrated.Username != "admin" || !auth.CheckPassword(migrated.PasswordHash, "migration-password") {
		t.Fatal("migration changed user identity or reset credentials")
	}
	var oldItems int64
	db.Model(&model.WatchlistItem{}).Where("user_id = ?", 2).Count(&oldItems)
	if oldItems != 1 {
		t.Fatal("legacy watchlist lost during migration")
	}
	log := logger.NewLogger()
	jwtSvc := auth.NewJWTService("test-signing-key", time.Hour)
	h := NewAuthHandler(db, jwtSvc, nil, fakeCodeService{}, fakeCodeService{}, log)
	r := gin.New()
	r.POST("/login", h.Login)
	r.POST("/register", h.Register)
	r.POST("/wechat", h.WeChatLogin)
	r.GET("/me", middleware.Auth(jwtSvc), h.GetMe)
	call := func(path, username, password string) *httptest.ResponseRecorder {
		payload := map[string]string{"username": username, "password": password, "code": "MOCK_WEB_LOGIN"}
		if path == "/register" {
			payload["email"] = "new-account@example.com"
			payload["code"] = "123456"
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, name := range []string{"admin", "does-not-exist"} {
		if response := call("/login", name, "wrong-password"); response.Code != 401 {
			t.Fatalf("incorrect credentials returned %d", response.Code)
		}
	}
	login := call("/login", " ADMIN ", "migration-password")
	if login.Code != 200 {
		t.Fatalf("login status %d", login.Code)
	}
	var session struct {
		Token string `json:"token"`
		User  struct {
			ID          uint   `json:"id"`
			Username    string `json:"username"`
			HasPassword bool   `json:"has_password"`
		} `json:"user"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	claims, err := jwtSvc.ParseToken(session.Token)
	if err != nil || claims.UserID != 2 || session.User.Username != "admin" || !session.User.HasPassword {
		t.Fatal("login did not preserve existing account")
	}
	if strings.Contains(login.Body.String(), "password_hash") || strings.Contains(login.Body.String(), "migration-password") {
		t.Fatal("credentials exposed in response")
	}
	registered := call("/register", " New_User ", "register-password")
	if registered.Code != 200 {
		t.Fatalf("registration status %d", registered.Code)
	}
	var created model.User
	db.Where("username=?", "new_user").First(&created)
	if created.ID == 0 || created.ID == 2 || !auth.CheckPassword(created.PasswordHash, "register-password") {
		t.Fatal("registration identity/hash incorrect")
	}
	var newItems int64
	db.Model(&model.WatchlistItem{}).Where("user_id=?", created.ID).Count(&newItems)
	if newItems != 0 {
		t.Fatal("new account inherited legacy watchlist")
	}
	if response := call("/register", "NEW_USER", "register-password"); response.Code != 409 {
		t.Fatal("duplicate account was accepted")
	}
	if response := call("/register", "other-user", "short"); response.Code != 400 {
		t.Fatal("short password was accepted")
	}
	if response := call("/wechat", "admin", "unused"); response.Code != 501 {
		t.Fatal("mock WeChat bypass is still enabled")
	}
	if err := database.BootstrapPasswordAccount(db, config.AccountBootstrapConfig{UserID: 999, Username: "missing", Password: "missing-password"}); err == nil {
		t.Fatal("bootstrap silently created or replaced an account")
	}
	if err := database.BootstrapPasswordAccount(db, config.AccountBootstrapConfig{UserID: created.ID, Username: "admin", Password: "other-password"}); err == nil {
		t.Fatal("bootstrap overwrote another account")
	}
	otherLegacy := model.User{Username: "legacy_other", Email: "other@example.invalid", Preferences: model.JSONB{}}
	db.Create(&otherLegacy)
	if err := database.BootstrapPasswordAccount(db, config.AccountBootstrapConfig{UserID: otherLegacy.ID, Username: "admin", Password: "other-password"}); err == nil {
		t.Fatal("bootstrap reused another account's username")
	}
	var unchanged model.User
	db.First(&unchanged, otherLegacy.ID)
	if unchanged.Username != "legacy_other" || unchanged.PasswordHash != "" {
		t.Fatal("failed bootstrap partially changed account")
	}
}

func TestAccountCannotReadDeleteOrMessageAnotherUsersConversation(t *testing.T) {
	db := passwordTestDB(t)
	user := model.User{Username: "owner", Email: "owner@example.invalid", Preferences: model.JSONB{}}
	db.Create(&user)
	conversation := model.Conversation{UserID: user.ID, AgentType: "group_chat", Title: "Private discussion", Status: model.ConversationStatusActive}
	db.Create(&conversation)
	log := logger.NewLogger()
	svc := service.NewConversationService(repository.NewConversationRepository(db), repository.NewMessageRepository(db), nil, nil, log)
	h := NewConversationHandler(svc, log)
	group := NewGroupChatHandler(svc, log)
	ownerRouter := gin.New()
	ownerRouter.Use(func(c *gin.Context) { c.Set("userID", user.ID); c.Next() })
	ownerRouter.GET("/conversations/:id", h.GetConversation)
	ownerResponse := httptest.NewRecorder()
	ownerRouter.ServeHTTP(ownerResponse, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/conversations/%d", conversation.ID), nil))
	if ownerResponse.Code != http.StatusOK {
		t.Fatal("owner can no longer read their own conversation")
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", user.ID+1); c.Next() })
	r.GET("/conversations/:id", h.GetConversation)
	r.DELETE("/conversations/:id", h.DeleteConversation)
	r.POST("/messages", h.SendMessage)
	r.POST("/messages/stream", h.SendMessageStream)
	r.POST("/messages/group-chat/stream", group.SendGroupChatStream)
	for _, target := range []struct{ method, path string }{{"GET", fmt.Sprintf("/conversations/%d", conversation.ID)}, {"DELETE", fmt.Sprintf("/conversations/%d", conversation.ID)}, {"POST", "/messages"}, {"POST", "/messages/stream"}, {"POST", "/messages/group-chat/stream"}} {
		body := fmt.Sprintf(`{"conversation_id":%d,"content":"must not be saved"}`, conversation.ID)
		req := httptest.NewRequest(target.method, target.path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 404 || strings.Contains(w.Header().Get("Content-Type"), "event-stream") {
			t.Errorf("%s %s was not rejected before streaming: %d", target.method, target.path, w.Code)
		}
	}
	var count int64
	db.Model(&model.Message{}).Count(&count)
	if count != 0 {
		t.Fatal("unauthorized request wrote a message")
	}
	var saved model.Conversation
	db.First(&saved, conversation.ID)
	if saved.Status != model.ConversationStatusActive {
		t.Fatal("unauthorized request deleted conversation")
	}
}

func TestPasswordAuthRateLimits(t *testing.T) {
	var limiter authLimit
	for i := 0; i < 5; i++ {
		if !limiter.allow("register:test-client", 5) {
			t.Fatal("allowed attempts rejected")
		}
	}
	if limiter.allow("register:test-client", 5) {
		t.Fatal("registration rate limit not applied")
	}
	if !limiter.allow("register:another-client", 5) {
		t.Fatal("another client was blocked")
	}
}
