package service

// GetWechatAuthURLRequest 获取微信授权URL请求
type GetWechatAuthURLRequest struct {
	RedirectURI string `json:"redirectUri" form:"redirectUri" binding:"required,url"`
	Scope      string `json:"scope" form:"scope" binding:"omitempty,oneof=snsapi_base snsapi_userinfo"`
	State      string `json:"state" form:"state"`
}

// GetWechatAuthURLResponse 获取微信授权URL响应
type GetWechatAuthURLResponse struct {
	URL string `json:"url"`
}

// WechatLoginRequest 微信登录请求
type WechatLoginRequest struct {
	Code string `json:"code" binding:"required"`
}

// WechatLoginResponse 微信登录响应
type WechatLoginResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresIn"`
	RememberMe bool   `json:"rememberMe"`
	UserID     int64  `json:"userId,omitempty"`
}

// WechatUserInfo 微信用户信息
type WechatUserInfo struct {
	OpenID    string `json:"openid"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"headimgurl"`
	UnionID   string `json:"unionid,omitempty"`
}

// WechatTokenResponse 微信Token响应
type WechatTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID      string `json:"openid"`
	Scope       string `json:"scope"`
	UnionID     string `json:"unionid,omitempty"`
}
