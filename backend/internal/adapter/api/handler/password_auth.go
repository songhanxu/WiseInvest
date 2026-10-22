package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"gorm.io/gorm"
	ormlog "gorm.io/gorm/logger"
)

type passwordLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// The limiter bounds password hashing and login attempts; entries expire together.
type authLimit struct {
	mu     sync.Mutex
	until  time.Time
	counts map[string]int
}

func (l *authLimit) allow(key string, maximum int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().After(l.until) {
		l.until = time.Now().Add(time.Minute)
		l.counts = map[string]int{}
	}
	if l.counts[key] >= maximum || len(l.counts) >= 10000 {
		return false
	}
	l.counts[key]++
	return true
}

func (h *AuthHandler) passwordRequest(c *gin.Context, registration bool) (passwordLoginRequest, bool) {
	var req passwordLoginRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写账号和密码"})
		return req, false
	}
	username, err := auth.NormalizeUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return req, false
	}
	req.Username = username
	if err := auth.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return req, false
	}
	key := "login:" + c.ClientIP()
	maximum := 30
	if registration {
		key = "register:" + c.ClientIP()
		maximum = 5
	}
	if !h.passwordLimits.allow(key, maximum) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试过于频繁，请一分钟后重试"})
		return req, false
	}
	return req, true
}

func (h *AuthHandler) Login(c *gin.Context) {
	req, ok := h.passwordRequest(c, false)
	if !ok {
		return
	}
	var user model.User
	err := h.db.WithContext(c.Request.Context()).Session(&gorm.Session{Logger: ormlog.Default.LogMode(ormlog.Silent)}).Where("LOWER(username) = ?", req.Username).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "登录服务暂不可用"})
		return
	}
	hash := user.PasswordHash
	if err != nil || !auth.HasPassword(hash) {
		hash = h.dummyPasswordHash
	}
	passwordMatches := auth.CheckPassword(hash, req.Password)
	if err != nil || !passwordMatches || !auth.HasPassword(user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账号或密码不正确"})
		return
	}
	h.passwordSession(c, &user, http.StatusOK)
}

func (h *AuthHandler) Register(c *gin.Context) {
	req, ok := h.passwordRequest(c, true)
	if !ok {
		return
	}
	db := h.db.WithContext(c.Request.Context()).Session(&gorm.Session{Logger: ormlog.Default.LogMode(ormlog.Silent)})
	var count int64
	if err := db.Unscoped().Model(&model.User{}).Where("LOWER(username) = ?", req.Username).Count(&count).Error; err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "注册服务暂不可用"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "这个账号已被使用"})
		return
	}
	// Reserve the requested administrator name until the existing account migrates.
	if req.Username == "admin" {
		c.JSON(http.StatusConflict, gin.H{"error": "这个账号已保留"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "注册服务暂不可用"})
		return
	}
	user := model.User{Username: req.Username, Email: "account_" + hex.EncodeToString(random[:]) + "@accounts.invalid", PasswordHash: hash, DisplayName: req.Username, Preferences: model.JSONB{}}
	if err := db.Create(&user).Error; err != nil {
		// Concurrent requests for the same normalized username must not create duplicates.
		var conflict int64
		db.Unscoped().Model(&model.User{}).Where("LOWER(username) = ?", req.Username).Count(&conflict)
		if conflict > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "这个账号已被使用"})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "注册服务暂不可用"})
		}
		return
	}
	h.passwordSession(c, &user, http.StatusCreated)
}

func (h *AuthHandler) passwordSession(c *gin.Context, user *model.User, status int) {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	token, err := h.jwtSvc.GenerateToken(user.ID, phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录会话创建失败"})
		return
	}
	c.JSON(status, gin.H{"token": token, "user": h.userResponse(user)})
}
