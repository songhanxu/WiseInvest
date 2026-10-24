package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// WeChatService handles WeChat OAuth flow
type WeChatService struct {
	appID         string
	appSecret     string
	miniAppID     string
	miniAppSecret string
	client        *http.Client
	apiBase       string
}

// MiniProgramSession is returned by the mini-program code2Session API.
type MiniProgramSession struct {
	OpenID     string `json:"openid"`
	UnionID    string `json:"unionid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
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
	return &WeChatService{appID: appID, appSecret: appSecret, client: &http.Client{Timeout: 8 * time.Second}, apiBase: "https://api.weixin.qq.com"}
}

// ConfigureMiniProgram configures the separate mini-program AppID and secret.
func (s *WeChatService) ConfigureMiniProgram(appID, appSecret string) {
	s.miniAppID, s.miniAppSecret = appID, appSecret
}

func (s *WeChatService) IsMiniProgramConfigured() bool {
	return s.miniAppID != "" && s.miniAppSecret != "" &&
		s.miniAppID != "your_mini_program_app_id_here" &&
		s.miniAppSecret != "your_mini_program_app_secret_here"
}

// ExchangeMiniProgramCode validates a wx.login code server-side. SessionKey is
// intentionally not returned to clients or stored in the application database.
func (s *WeChatService) ExchangeMiniProgramCode(ctx context.Context, code string) (*MiniProgramSession, error) {
	query := url.Values{
		"appid": {s.miniAppID}, "secret": {s.miniAppSecret},
		"js_code": {code}, "grant_type": {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBase+"/sns/jscode2session?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat mini-program code exchange failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wechat mini-program response status %d", resp.StatusCode)
	}
	var session MiniProgramSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("wechat mini-program response decode failed: %w", err)
	}
	if session.ErrCode != 0 || session.OpenID == "" {
		return nil, fmt.Errorf("wechat mini-program error %d: %s", session.ErrCode, session.ErrMsg)
	}
	return &session, nil
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
	query := url.Values{"appid": {s.appID}, "secret": {s.appSecret}, "code": {code}, "grant_type": {"authorization_code"}}
	resp, err := s.client.Get(s.apiBase + "/sns/oauth2/access_token?" + query.Encode()) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("wechat code exchange failed")
	}
	defer resp.Body.Close()

	var result WeChatTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("wechat response decode failed: %w", err)
	}
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat error %d: %s", result.ErrCode, result.ErrMsg)
	}
	if result.OpenID == "" || result.AccessToken == "" {
		return nil, fmt.Errorf("wechat OAuth response missing identity")
	}
	return &result, nil
}

// GetUserInfo fetches WeChat user profile information
func (s *WeChatService) GetUserInfo(accessToken, openID string) (*WeChatUserInfo, error) {
	query := url.Values{"access_token": {accessToken}, "openid": {openID}, "lang": {"zh_CN"}}
	resp, err := s.client.Get(s.apiBase + "/sns/userinfo?" + query.Encode()) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("wechat get user info failed")
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
