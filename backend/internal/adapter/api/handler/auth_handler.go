package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	db                *gorm.DB
	jwtSvc            *auth.JWTService
	wechatSvc         *auth.WeChatService
	smsSvc            oneTimeCodeService
	emailSvc          oneTimeCodeService
	log               *logger.Logger
	passwordLimits    authLimit
	dummyPasswordHash string
}

type oneTimeCodeService interface {
	IsConfigured() bool
	SendCode(ctx context.Context, target, ip string) error
	VerifyCode(ctx context.Context, target, code string) (bool, error)
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(
	db *gorm.DB,
	jwtSvc *auth.JWTService,
	wechatSvc *auth.WeChatService,
	smsSvc oneTimeCodeService,
	emailSvc oneTimeCodeService,
	log *logger.Logger,
) *AuthHandler {
	dummyHash, _ := auth.HashPassword("invalid-account-placeholder")
	return &AuthHandler{
		db:                db,
		jwtSvc:            jwtSvc,
		wechatSvc:         wechatSvc,
		smsSvc:            smsSvc,
		emailSvc:          emailSvc,
		log:               log,
		dummyPasswordHash: dummyHash,
	}
}

type emailCredentials struct {
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Password   string `json:"password" binding:"required"`
	Username   string `json:"username"`
	Code       string `json:"code"`
	Identifier string `json:"identifier"`
}

var accountNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

// Register creates one account with a verified email or phone and a password.
func (h *AuthHandler) Register(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req emailCredentials
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入账号和密码"})
		return
	}
	if !h.passwordLimits.allow("register:"+c.ClientIP(), 10) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试过于频繁，请一分钟后重试"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	phone := strings.TrimSpace(req.Phone)
	if (email == "") == (phone == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择邮箱或手机号注册"})
		return
	}
	if len(req.Password) < 10 || len(req.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码长度应为 10 到 72 个字符"})
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !accountNamePattern.MatchString(username) || mainlandPhonePattern.MatchString(username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号名须为 3 到 32 位字母、数字、下划线、点或短横线"})
		return
	}
	if username == "admin" {
		c.JSON(http.StatusConflict, gin.H{"error": "这个账号已保留"})
		return
	}
	if !smsCodePattern.MatchString(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 6 位验证码"})
		return
	}
	var verifier oneTimeCodeService
	target := email
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || len(email) > 254 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的邮箱地址"})
			return
		}
		verifier = h.emailSvc
	} else {
		if !mainlandPhonePattern.MatchString(phone) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的中国大陆手机号"})
			return
		}
		verifier = h.smsSvc
		target = phone
		email = "phone_" + phone + "@phone.local"
	}
	if verifier == nil || !verifier.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "验证码服务尚未开通"})
		return
	}
	var existing model.User
	if h.db.Where("LOWER(username) = ? OR email = ?", username, email).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "账号名或联系方式已被使用"})
		return
	}
	if phone != "" && h.db.Where("phone = ?", phone).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "手机号已被使用"})
		return
	}
	ok, err := verifier.VerifyCode(c.Request.Context(), target, req.Code)
	if errors.Is(err, auth.ErrSMSRateLimited) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "验证次数过多，请重新获取验证码"})
		return
	}
	if err != nil {
		h.log.Errorf("Registration code verification failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "验证码验证失败"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码错误或已过期"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.log.Errorf("Password hashing failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败，请稍后重试"})
		return
	}
	user := model.User{Username: username, Email: email, PasswordHash: string(hash), DisplayName: username, Preferences: model.JSONB{}}
	if phone != "" {
		user.Phone = &phone
	}
	// Never put the password hash in SQL logs, including constraint errors.
	if err := h.db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}).Create(&user).Error; err != nil {
		var existing model.User
		if h.db.Where("LOWER(username) = ? OR email = ? OR phone = ?", username, email, phone).First(&existing).Error == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "账号名或联系方式已被使用"})
			return
		}
		h.log.Errorf("Account registration failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败，请稍后重试"})
		return
	}
	h.respondWithToken(c, &user)
}

// Login authenticates the same password with account name, email or phone.
func (h *AuthHandler) Login(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req emailCredentials
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入账号和密码"})
		return
	}
	if !h.passwordLimits.allow("login:"+c.ClientIP(), 30) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试过于频繁，请一分钟后重试"})
		return
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Username)
	}
	if identifier == "" {
		identifier = strings.TrimSpace(req.Email)
	}
	if identifier == "" {
		identifier = strings.TrimSpace(req.Phone)
	}
	identifier = strings.ToLower(identifier)
	if identifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入账号、邮箱或手机号"})
		return
	}
	var user model.User
	query := "LOWER(username) = ?"
	if strings.Contains(identifier, "@") {
		query = "email = ?"
	} else if mainlandPhonePattern.MatchString(identifier) {
		query = "phone = ?"
	}
	err := h.db.Where(query, identifier).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		h.log.Errorf("Account lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败，请稍后重试"})
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
	h.respondWithToken(c, &user)
}

func (h *AuthHandler) respondWithToken(c *gin.Context, user *model.User) {
	phone := ""
	if user.Phone != nil {
		phone = *user.Phone
	}
	token, err := h.jwtSvc.GenerateToken(user.ID, phone)
	if err != nil {
		h.log.Errorf("JWT generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": h.userResponse(user)})
}

// ─── WeChat Login ─────────────────────────────────────────────────────────────

type wechatLoginRequest struct {
	Code string `json:"code" binding:"required"`
}

func mockAuthEnabled() bool {
	return os.Getenv("SERVER_MODE") == "debug" && os.Getenv("ALLOW_MOCK_AUTH") == "true"
}

// WeChatLogin exchanges a WeChat authorization code for a JWT token.
// POST /api/v1/auth/wechat/login
//
// Unconfigured WeChat login is disabled; it must never bypass password login.

func (h *AuthHandler) WeChatLogin(c *gin.Context) {
	var req wechatLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	if h.wechatSvc == nil || !h.wechatSvc.IsConfigured() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "请使用账号密码登录"})
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

	}

	if err != nil {
		h.log.Errorf("findOrCreateWeChatUser failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation failed"})
		return
	}

	h.respondWithWeChatToken(c, user)
}

// MiniProgramLogin exchanges a wx.login code with code2Session. Mobile app
// OAuth codes use a different WeChat API and remain on /wechat/login.
func (h *AuthHandler) MiniProgramLogin(c *gin.Context) {
	var req wechatLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}
	var openID, unionID string
	if h.wechatSvc.IsMiniProgramConfigured() {
		session, err := h.wechatSvc.ExchangeMiniProgramCode(c.Request.Context(), req.Code)
		if err != nil {
			h.log.Errorf("Mini-program login failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "微信登录失败，请重试"})
			return
		}
		openID, unionID = session.OpenID, session.UnionID
	} else if mockAuthEnabled() && strings.HasPrefix(req.Code, "MOCK_") {
		openID = "dev_mock_mini_openid"
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "小程序微信登录暂不可用"})
		return
	}
	user, err := h.findOrCreateMiniProgramUser(openID, unionID)
	if err != nil {
		h.log.Errorf("Mini-program account lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败，请稍后重试"})
		return
	}
	h.respondWithWeChatToken(c, user)
}

func (h *AuthHandler) respondWithWeChatToken(c *gin.Context, user *model.User) {
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
	c.JSON(http.StatusOK, gin.H{"token": token, "needs_phone_binding": user.Phone == nil, "user": h.userResponse(user)})
}

func (h *AuthHandler) findOrCreateMiniProgramUser(openID, unionID string) (*model.User, error) {
	var user model.User
	err := h.db.Where("wechat_mini_open_id = ?", openID).First(&user).Error
	if err == nil {
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if unionID != "" {
		err = h.db.Where("wechat_union_id = ?", unionID).First(&user).Error
		if err == nil {
			if err := h.db.Model(&user).Update("wechat_mini_open_id", openID).Error; err != nil {
				return nil, err
			}
			user.WeChatMiniOpenID = &openID
			return &user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	user = model.User{
		Username: "wxmini_" + openID, Email: "wxmini_" + openID + "@wechat.local",
		WeChatMiniOpenID: &openID, WeChatUnionID: unionID,
		DisplayName: "微信用户", Preferences: model.JSONB{},
	}
	if err := h.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (h *AuthHandler) findOrCreateWeChatUser(openID, unionID string, info *auth.WeChatUserInfo) (*model.User, error) {
	var user model.User
	err := h.db.Where("wechat_open_id = ?", openID).First(&user).Error
	if err == nil {
		// Existing user — refresh WeChat profile fields
		if err := h.db.Model(&user).Updates(map[string]interface{}{
			"wechat_nickname": info.Nickname,
			"wechat_avatar":   info.HeadImgURL,
		}).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if unionID != "" {
		err = h.db.Where("wechat_union_id = ?", unionID).First(&user).Error
		if err == nil {
			if err := h.db.Model(&user).Updates(map[string]interface{}{
				"wechat_open_id": openID, "wechat_nickname": info.Nickname, "wechat_avatar": info.HeadImgURL,
			}).Error; err != nil {
				return nil, err
			}
			user.WeChatOpenID = &openID
			return &user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	// Use the full verified OpenID so two distinct users cannot share an alias.
	username := fmt.Sprintf("wx_%s", openID)
	email := fmt.Sprintf("wx_%s@wechat.local", openID)
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

var mainlandPhonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
var smsCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

type emailCodeRequest struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code"`
}

// SendEmailCode supports both verified email registration and code login.
func (h *AuthHandler) SendEmailCode(c *gin.Context) {
	var req emailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入邮箱地址"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的邮箱地址"})
		return
	}
	if h.emailSvc == nil || !h.emailSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱验证码服务尚未开通"})
		return
	}
	requestIP := c.GetHeader("X-Real-IP")
	if requestIP == "" {
		requestIP = c.Request.RemoteAddr
	}
	if err := h.emailSvc.SendCode(c.Request.Context(), email, requestIP); err != nil {
		if errors.Is(err, auth.ErrEmailUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱验证码服务尚未开通"})
			return
		}
		if errors.Is(err, auth.ErrSMSRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "发送过于频繁，请稍后重试"})
			return
		}
		h.log.Errorf("Email code send failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "验证码发送失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "验证码已发送"})
}

func (h *AuthHandler) EmailLogin(c *gin.Context) {
	var req emailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || !smsCodePattern.MatchString(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入邮箱和 6 位验证码"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的邮箱地址"})
		return
	}
	if h.emailSvc == nil || !h.emailSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱验证码服务尚未开通"})
		return
	}
	ok, err := h.emailSvc.VerifyCode(c.Request.Context(), email, req.Code)
	if errors.Is(err, auth.ErrSMSRateLimited) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "验证次数过多，请重新获取验证码"})
		return
	}
	if err != nil {
		h.log.Errorf("Email code verification failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "验证失败，请稍后重试"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码错误或已过期"})
		return
	}
	var user model.User
	if err := h.db.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "该邮箱尚未注册"})
			return
		}
		h.log.Errorf("Email account lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败，请稍后重试"})
		return
	}
	h.respondWithToken(c, &user)
}

// BindEmail attaches a verified email to the currently authenticated account.
func (h *AuthHandler) BindEmail(c *gin.Context) {
	var req emailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || !smsCodePattern.MatchString(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入邮箱和 6 位验证码"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的邮箱地址"})
		return
	}
	if h.emailSvc == nil || !h.emailSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "邮箱验证码服务尚未开通"})
		return
	}
	var existing model.User
	if err := h.db.Where("email = ? AND id != ?", email, c.GetUint("userID")).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已绑定其他账户"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		h.log.Errorf("Email binding lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "绑定失败，请稍后重试"})
		return
	}
	ok, err := h.emailSvc.VerifyCode(c.Request.Context(), email, req.Code)
	if errors.Is(err, auth.ErrSMSRateLimited) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "验证次数过多，请重新获取验证码"})
		return
	}
	if err != nil {
		h.log.Errorf("Email binding verification failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "绑定失败，请稍后重试"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码错误或已过期"})
		return
	}
	var user model.User
	if err := h.db.First(&user, c.GetUint("userID")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err := h.db.Model(&user).Update("email", email).Error; err != nil {
		h.log.Errorf("Email binding failed: %v", err)
		c.JSON(http.StatusConflict, gin.H{"error": "邮箱绑定失败，请检查是否已被使用"})
		return
	}
	user.Email = email
	h.respondWithToken(c, &user)
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
	if !mainlandPhonePattern.MatchString(phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的中国大陆手机号"})
		return
	}

	// Nginx overwrites X-Real-IP with its peer's address. Prefer it over
	// X-Forwarded-For, which may contain untrusted client-supplied values.
	requestIP := c.GetHeader("X-Real-IP")
	if requestIP == "" {
		requestIP = c.Request.RemoteAddr
	}
	if err := h.smsSvc.SendCode(c.Request.Context(), phone, requestIP); err != nil {
		if errors.Is(err, auth.ErrSMSUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "短信服务尚未开通，请使用其他登录方式"})
			return
		}
		if errors.Is(err, auth.ErrSMSRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "发送过于频繁，请稍后重试"})
			return
		}
		h.log.Errorf("SMS send failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification code"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Verification code sent"})
}

type bindPhoneRequest struct {
	Phone  string `json:"phone" binding:"required"`
	Code   string `json:"code" binding:"required"`
	Create *bool  `json:"create"`
}

// PhoneLogin signs in an existing phone owner or creates a phone account after
// a one-time code has been verified.
func (h *AuthHandler) PhoneLogin(c *gin.Context) {
	if !h.smsSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "短信登录尚未开通"})
		return
	}
	var req bindPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入手机号和验证码"})
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if !mainlandPhonePattern.MatchString(phone) || !smsCodePattern.MatchString(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号或验证码格式不正确"})
		return
	}
	ok, err := h.smsSvc.VerifyCode(c.Request.Context(), phone, req.Code)
	if errors.Is(err, auth.ErrSMSRateLimited) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "验证次数过多，请重新获取验证码"})
		return
	}
	if err != nil {
		h.log.Errorf("Phone code verification failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "验证失败，请稍后重试"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "验证码错误或已过期"})
		return
	}
	var user model.User
	err = h.db.Where("phone = ?", phone).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if req.Create != nil && !*req.Create {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "该手机号尚未注册"})
			return
		}
		user = model.User{
			Username: "phone_" + phone, Email: "phone_" + phone + "@phone.local",
			Phone: &phone, DisplayName: "慧投用户", Preferences: model.JSONB{},
		}
		err = h.db.Create(&user).Error
	}
	if err != nil {
		h.log.Errorf("Phone account lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败，请稍后重试"})
		return
	}
	h.respondWithToken(c, &user)
}

// BindPhone verifies the SMS code and binds the phone number to the authenticated user.
// POST /api/v1/auth/phone/bind
func (h *AuthHandler) BindPhone(c *gin.Context) {
	if !h.smsSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "短信服务尚未开通"})
		return
	}
	userID := c.GetUint("userID")

	var req bindPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone and code are required"})
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if !mainlandPhonePattern.MatchString(phone) || !smsCodePattern.MatchString(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号或验证码格式不正确"})
		return
	}

	ok, err := h.smsSvc.VerifyCode(c.Request.Context(), phone, req.Code)
	if errors.Is(err, auth.ErrSMSRateLimited) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "验证次数过多，请重新获取验证码"})
		return
	}
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
	ID          uint   `json:"id"`
	Username    string `json:"username"`
	HasPassword bool   `json:"has_password"`
	Email       string `json:"email,omitempty"`

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
		ID:          u.ID,
		Username:    u.Username,
		HasPassword: auth.HasPassword(u.PasswordHash),
		Email:       publicEmail(u.Email),

		DisplayName:    u.DisplayName,
		Avatar:         avatar,
		WeChatNickname: u.WeChatNickname,
		WeChatAvatar:   u.WeChatAvatar,
		Phone:          phone,
	}
}

func publicEmail(email string) string {
	if strings.HasSuffix(email, "@phone.local") || strings.HasSuffix(email, "@wechat.local") || strings.HasSuffix(email, "@accounts.invalid") {
		return ""
	}
	return email
}
