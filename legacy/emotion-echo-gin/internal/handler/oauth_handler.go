package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// OAuthHandler OAuth 处理器
type OAuthHandler struct {
	wechatOAuthService *service.WechatOAuthService
}

// NewOAuthHandler 创建 OAuth 处理器
func NewOAuthHandler(wechatOAuthService *service.WechatOAuthService) *OAuthHandler {
	return &OAuthHandler{
		wechatOAuthService: wechatOAuthService,
	}
}

// GetWechatAuthURL 获取微信授权 URL
// GET /auth/oauth/wechat/url?redirectUri=xxx&scope=snsapi_userinfo&state=xxx
func (h *OAuthHandler) GetWechatAuthURL(c *gin.Context) {
	var req service.GetWechatAuthURLRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	resp := h.wechatOAuthService.GetAuthURL(&req)
	response.Success(c, resp)
}

// WechatLogin 微信登录
// POST /auth/oauth/wechat/login
func (h *OAuthHandler) WechatLogin(c *gin.Context) {
	var req service.WechatLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	resp, refreshToken, err := h.wechatOAuthService.LoginByCode(c.Request.Context(), req.Code)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	// 设置 RefreshToken Cookie
	// 微信登录默认记住（30天）
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refreshToken",
		refreshToken,
		30*24*3600, // 30天
		"/api/v1",
		"",
		true,  // Secure
		true,  // HttpOnly
	)

	response.Success(c, resp)
}
