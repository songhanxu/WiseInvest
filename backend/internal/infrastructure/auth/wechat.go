package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WeChatService handles WeChat OAuth flow
type WeChatService struct {
	appID     string
	appSecret string
}

// WeChatTokenResponse is the response from WeChat code exchange
type WeChatTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	Scope        string `json:"scope"`
	UnionID      string `json:"unionid"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

// WeChatUserInfo is the user info returned from WeChat
type WeChatUserInfo struct {
	OpenID     string `json:"openid"`
	Nickname   string `json:"nickname"`
	Sex        int    `json:"sex"`
	Province   string `json:"province"`
	City       string `json:"city"`
	Country    string `json:"country"`
	HeadImgURL string `json:"headimgurl"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// NewWeChatService creates a new WeChat service
func NewWeChatService(appID, appSecret string) *WeChatService {
	return &WeChatService{appID: appID, appSecret: appSecret}
}

// IsConfigured returns true when real WeChat credentials are provided
func (s *WeChatService) IsConfigured() bool {
	return s.appID != "" &&
		s.appID != "your_wechat_app_id_here" &&
		s.appSecret != "" &&
		s.appSecret != "your_wechat_app_secret_here"
}

// ExchangeCode exchanges a WeChat authorization code for an access token and openid
func (s *WeChatService) ExchangeCode(code string) (*WeChatTokenResponse, error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/oauth2/access_token?appid=%s&secret=%s&code=%s&grant_type=authorization_code",
		s.appID, s.appSecret, code,
	)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("wechat code exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var result WeChatTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("wechat response decode failed: %w", err)
	}
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return &result, nil
}

// GetUserInfo fetches WeChat user profile information
func (s *WeChatService) GetUserInfo(accessToken, openID string) (*WeChatUserInfo, error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/userinfo?access_token=%s&openid=%s&lang=zh_CN",
		accessToken, openID,
	)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("wechat get user info failed: %w", err)
	}
	defer resp.Body.Close()

	var result WeChatUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("wechat user info decode failed: %w", err)
	}
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat user info error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return &result, nil
}
