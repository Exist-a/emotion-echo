package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	cfg           *config.Config
	authService   *service.AuthService
	verifyService *service.VerifyService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(cfg *config.Config, authService *service.AuthService, verifyService *service.VerifyService) *AuthHandler {
	return &AuthHandler{
		cfg:           cfg,
		authService:   authService,
		verifyService: verifyService,
	}
}

// setRefreshTokenCookie 设置 RefreshToken Cookie（根据环境自动调整安全属性）
func (h *AuthHandler) setRefreshTokenCookie(c *gin.Context, token string, maxAge int) {
	secure := true
	sameSite := http.SameSiteStrictMode
	
	// 开发环境放宽限制
	if h.cfg.Server.Mode == "debug" {
		secure = false
		sameSite = http.SameSiteLaxMode
	}
	
	c.SetSameSite(sameSite)
	c.SetCookie(
		"refreshToken",
		token,
		maxAge,
		"/api/v1",
		"",
		secure,
		true, // HttpOnly
	)
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	resp, refreshToken, err := h.authService.Register(c.Request.Context(), &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	// 设置 RefreshToken Cookie
	h.setRefreshTokenCookie(c, refreshToken, 7*24*3600)

	response.Success(c, resp)
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	resp, refreshToken, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	// 设置 RefreshToken Cookie
	// 记住我：30天，不记住我：Session Cookie（关闭浏览器失效）
	maxAge := 0 // Session Cookie
	if req.RememberMe {
		maxAge = 30 * 24 * 3600 // 30天
	}
	h.setRefreshTokenCookie(c, refreshToken, maxAge)

	response.Success(c, resp)
}

// Refresh 刷新 Token（支持 Token Rotation）
func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refreshToken")
	if err != nil {
		response.ErrorWithCode(c, errors.ErrTokenInvalid, "refresh token not found")
		return
	}

	resp, newRefreshToken, err := h.authService.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	// Token Rotation：设置新的 refreshToken cookie
	maxAge := 0
	if resp.RememberMe {
		maxAge = 30 * 24 * 3600
	}
	h.setRefreshTokenCookie(c, newRefreshToken, maxAge)

	response.Success(c, resp)
}

// Logout 用户登出
func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, err := c.Cookie("refreshToken")
	if err == nil && refreshToken != "" {
		h.authService.Logout(c.Request.Context(), refreshToken)
	}

	// 清除 Cookie
	h.setRefreshTokenCookie(c, "", -1)

	response.Success(c, nil)
}

// ResetPassword 重置密码
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req service.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	if err := h.authService.ResetPassword(c.Request.Context(), &req); err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, nil)
}

// SendVerifyCode 发送验证码
func (h *AuthHandler) SendVerifyCode(c *gin.Context) {
	var req service.SendVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	if err := h.verifyService.SendVerifyCode(c.Request.Context(), &req); err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, nil)
}
