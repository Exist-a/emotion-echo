package service

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"emotion-echo-gin/internal/config"
)

// WechatAPI 微信API服务
type WechatAPI struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewWechatAPI 创建微信API服务
func NewWechatAPI(cfg *config.Config) *WechatAPI {
	return &WechatAPI{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetAuthURL 获取授权URL
func (s *WechatAPI) GetAuthURL(req *GetWechatAuthURLRequest) *GetWechatAuthURLResponse {
	scope := req.Scope
	if scope == "" {
		scope = "snsapi_userinfo"
	}

	state := req.State
	if state == "" {
		state = generateRandomState()
	}

	authURL := fmt.Sprintf(
		"https://open.weixin.qq.com/connect/oauth2/authorize?appid=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s#wechat_redirect",
		s.cfg.OAuth.WechatAppID,
		req.RedirectURI,
		scope,
		state,
	)

	return &GetWechatAuthURLResponse{URL: authURL}
}

// ExchangeCodeForToken 通过code换取access_token
func (s *WechatAPI) ExchangeCodeForToken(code string) (*WechatTokenResponse, error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/oauth2/access_token?appid=%s&secret=%s&code=%s&grant_type=authorization_code",
		s.cfg.OAuth.WechatAppID,
		s.cfg.OAuth.WechatAppSecret,
		code,
	)

	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp WechatTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// GetUserInfo 获取用户信息
func (s *WechatAPI) GetUserInfo(accessToken, openID string) (*WechatUserInfo, error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/userinfo?access_token=%s&openid=%s",
		accessToken,
		openID,
	)

	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var userInfo WechatUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

// generateRandomState 生成随机state
func generateRandomState() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
