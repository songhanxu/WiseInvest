package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"gorm.io/gorm"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	db         *gorm.DB
	jwtSvc     *auth.JWTService
	wechatSvc  *auth.WeChatService
	smsSvc     *auth.SMSService
	log        *logger.Logger
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(
	db *gorm.DB,
	jwtSvc *auth.JWTService,
	wechatSvc *auth.WeChatService,
	smsSvc *auth.SMSService,
	log *logger.Logger,
) *AuthHandler {
	return &AuthHandler{
		db:        db,
		jwtSvc:    jwtSvc,
		wechatSvc: wechatSvc,
		smsSvc:    smsSvc,
		log:       log,
	}
}

// ─── WeChat Login ─────────────────────────────────────────────────────────────

type wechatLoginRequest struct {
	Code string `json:"code" binding:"required"`
}

// WeChatLogin exchanges a WeChat authorization code for a JWT token.
// POST /api/v1/auth/wechat/login
//
// Dev mode: when WECHAT_APP_ID is not configured, any code prefixed with
// "MOCK_" is accepted and a dev user is returned directly (no WeChat API call).
func (h *AuthHandler) WeChatLogin(c *gin.Context) {
	var req wechatLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	var user *model.User
	var err error

	if h.wechatSvc.IsConfigured() {
		// ── Production: real WeChat OAuth flow ───────────────────────────
		tokenResp, exchErr := h.wechatSvc.ExchangeCode(req.Code)
		if exchErr != nil {
			h.log.Errorf("WeChat code exchange failed: %v", exchErr)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "WeChat authentication failed"})
			return
		}

		userInfo, infoErr := h.wechatSvc.GetUserInfo(tokenResp.AccessToken, tokenResp.OpenID)
		if infoErr != nil {
			h.log.Errorf("WeChat user info failed: %v", infoErr)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Failed to get WeChat user info"})
			return
		}

		user, err = h.findOrCreateWeChatUser(tokenResp.OpenID, tokenResp.UnionID, userInfo)
	} else {
		// ── Dev mode: mock flow (no real WeChat credentials) ─────────────
		h.log.Warn("WECHAT_APP_ID not configured — using dev mock user")
		mockOpenID := "dev_mock_openid"
		mockInfo := &auth.WeChatUserInfo{
			OpenID:     mockOpenID,
			Nickname:   "开发测试用户",
			HeadImgURL: "",
		}
		user, err = h.findOrCreateWeChatUser(mockOpenID, "", mockInfo)
	}

	if err != nil {
		h.log.Errorf("findOrCreateWeChatUser failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation failed"})
		return
	}

	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	token, err := h.jwtSvc.GenerateToken(user.ID, phone)
	if err != nil {
		h.log.Errorf("JWT generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	needsPhoneBinding := user.Phone == nil
	c.JSON(http.StatusOK, gin.H{
		"token":               token,
		"needs_phone_binding": needsPhoneBinding,
		"user":                h.userResponse(user),
	})
}

func (h *AuthHandler) findOrCreateWeChatUser(openID, unionID string, info *auth.WeChatUserInfo) (*model.User, error) {
	var user model.User
	err := h.db.Where("wechat_open_id = ?", openID).First(&user).Error
	if err == nil {
		// Existing user — refresh WeChat profile fields
		h.db.Model(&user).Updates(map[string]interface{}{
			"wechat_nickname": info.Nickname,
			"wechat_avatar":   info.HeadImgURL,
		})
		return &user, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// New user — auto-generate username/email placeholders
	suffix := openID
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	username := fmt.Sprintf("wx_%s", suffix)
	email := fmt.Sprintf("wx_%s@wechat.local", suffix)
	displayName := info.Nickname
	if displayName == "" {
		displayName = "微信用户"
	}

	user = model.User{
		Username:       username,
		Email:          email,
		WeChatOpenID:   &openID,
		WeChatUnionID:  unionID,
		WeChatNickname: info.Nickname,
		WeChatAvatar:   info.HeadImgURL,
		DisplayName:    displayName,
		Avatar:         info.HeadImgURL,
		Preferences:    model.JSONB{},
	}
	if err := h.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// ─── Phone Binding ────────────────────────────────────────────────────────────

type sendCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
}

// SendPhoneCode sends an SMS verification code to the given phone number.
// POST /api/v1/auth/phone/send-code
func (h *AuthHandler) SendPhoneCode(c *gin.Context) {
	var req sendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if len(phone) < 11 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phone number"})
		return
	}

	if err := h.smsSvc.SendCode(c.Request.Context(), phone); err != nil {
		h.log.Errorf("SMS send failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification code"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Verification code sent"})
}

type bindPhoneRequest struct {
	Phone string `json:"phone" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

// BindPhone verifies the SMS code and binds the phone number to the authenticated user.
// POST /api/v1/auth/phone/bind
func (h *AuthHandler) BindPhone(c *gin.Context) {
	userID := c.GetUint("userID")

	var req bindPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone and code are required"})
		return
	}
	phone := strings.TrimSpace(req.Phone)

	ok, err := h.smsSvc.VerifyCode(c.Request.Context(), phone, req.Code)
	if err != nil {
		h.log.Errorf("SMS verify failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Verification failed"})
		return
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired verification code"})
		return
	}

	// Check phone not already registered to another user
	var existing model.User
	if h.db.Where("phone = ? AND id != ?", phone, userID).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Phone number already registered"})
		return
	}

	// Bind phone to current user
	if err := h.db.Model(&model.User{}).Where("id = ?", userID).Update("phone", phone).Error; err != nil {
		h.log.Errorf("Failed to bind phone: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to bind phone"})
		return
	}

	// Re-fetch user and issue a fresh token (now includes phone)
	var user model.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	token, err := h.jwtSvc.GenerateToken(user.ID, phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  h.userResponse(&user),
	})
}

// ─── Profile ──────────────────────────────────────────────────────────────────

// GetMe returns the authenticated user's profile.
// GET /api/v1/auth/me
func (h *AuthHandler) GetMe(c *gin.Context) {
	userID := c.GetUint("userID")

	var user model.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, h.userResponse(&user))
}

// ─── Helper ───────────────────────────────────────────────────────────────────

type userResponse struct {
	ID             uint   `json:"id"`
	DisplayName    string `json:"display_name"`
	Avatar         string `json:"avatar"`
	WeChatNickname string `json:"wechat_nickname,omitempty"`
	WeChatAvatar   string `json:"wechat_avatar,omitempty"`
	Phone          string `json:"phone,omitempty"`
}

func (h *AuthHandler) userResponse(u *model.User) userResponse {
	phone := ""
	if u.Phone != nil {
		phone = *u.Phone
	}
	avatar := u.Avatar
	if avatar == "" {
		avatar = u.WeChatAvatar
	}
	return userResponse{
		ID:             u.ID,
		DisplayName:    u.DisplayName,
		Avatar:         avatar,
		WeChatNickname: u.WeChatNickname,
		WeChatAvatar:   u.WeChatAvatar,
		Phone:          phone,
	}
}
