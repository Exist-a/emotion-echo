// Package handler — auth_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.32-34: auth handler（BFF 自有 mock 鉴权）
//
// 端点（前端 stores/user.ts 调用）：
//   POST /api/v1/auth/login              {username, password, rememberMe} → LoginData
//   POST /api/v1/auth/register           {username, password, verificationCode} → LoginData
//   POST /api/v1/auth/refresh            → LoginData（重新签发）
//   POST /api/v1/auth/logout             → {"success": true}
//   POST /api/v1/auth/verification-code  {username} → {"success": true}（mock）
//
// LoginData：{accessToken, expiresIn, user}
//   user：{userId, account, phone, nickname}
//
// mock 语义：任何非空 username/password 都签发 token（user_id 由 username hash 派生，
// 保证同一账号稳定映射到同一 user_id）。生产替换见 internal/auth/jwt.go 注释。
package handler

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
)

// LoginData 是登录/注册/refresh 的统一响应（前端 LoginData，types/api.ts）
type LoginData struct {
	AccessToken string       `json:"accessToken"`
	ExpiresIn   int64        `json:"expiresIn"`
	User        AuthUserInfo `json:"user"`
}

// AuthUserInfo 是 BFF 签发 JWT 后返回的用户信息（与前端 types/api.ts UserInfo 对齐：
// id 是 string，含 username/avatar/config 等前端依赖字段；真实用户信息来自 user-svc
// fetchUserInfo，此处为 mock 登录后的初始形状）。
type AuthUserInfo struct {
	ID        string         `json:"id"`
	Username  string         `json:"username"`
	Nickname  string         `json:"nickname"`
	Avatar    string         `json:"avatar"`
	Age       *int           `json:"age"`
	Config    map[string]any `json:"config"`
	CreatedAt string         `json:"createdAt"`
}

// AuthHandler 处理 /api/v1/auth/* 端点
type AuthHandler struct {
	jwt *auth.Manager
}

// NewAuthHandler 构造
func NewAuthHandler(mgr *auth.Manager) gin.HandlerFunc {
	h := &AuthHandler{jwt: mgr}
	return func(c *gin.Context) {
		switch c.Param("action") {
		case "login":
			h.login(c)
		case "register":
			h.register(c)
		case "refresh":
			h.refresh(c)
		case "logout":
			h.logout(c)
		case "verification-code":
			h.verificationCode(c)
		default:
			c.JSON(http.StatusNotFound, gin.H{"error": "auth endpoint not found"})
		}
	}
}

// stableUserID 从 username 派生稳定 user_id（同一账号 → 同一 user_id）
func stableUserID(username string) int64 {
	sum := sha256.Sum256([]byte("bff:" + username))
	// 取前 8 字节作为正 int64
	id := int64(binary.BigEndian.Uint64(sum[:8]))
	if id < 0 {
		id = -id
	}
	if id == 0 {
		id = 1
	}
	return id
}

func (h *AuthHandler) login(c *gin.Context) {
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"rememberMe"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: username and password are required"})
		return
	}
	userID := stableUserID(req.Username)
	c.JSON(http.StatusOK, h.buildLoginData(userID, req.Username))
}

func (h *AuthHandler) register(c *gin.Context) {
	var req struct {
		Username         string `json:"username"`
		Password         string `json:"password"`
		VerificationCode string `json:"verificationCode"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: username and password are required"})
		return
	}
	userID := stableUserID(req.Username)
	c.JSON(http.StatusOK, h.buildLoginData(userID, req.Username))
}

func (h *AuthHandler) refresh(c *gin.Context) {
	// mock：无 token 校验，直接重新签发（user 从 Authorization 解析，失败则用固定 id）
	var userID int64 = 1
	token := downstream.JWTFromContext(c.Request.Context())
	if token != "" {
		if uid, err := h.jwt.Parse(token); err == nil {
			userID = uid
		}
	}
	c.JSON(http.StatusOK, h.buildLoginData(userID, "user"))
}

func (h *AuthHandler) logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) verificationCode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *AuthHandler) buildLoginData(userID int64, username string) LoginData {
	token, _ := h.jwt.Sign(userID, username)
	expiresIn := int64(h.jwt.TTL() / time.Second)
	return LoginData{
		AccessToken: token,
		ExpiresIn:   expiresIn,
		User: AuthUserInfo{
			ID:        fmt.Sprintf("%d", userID),
			Username:  username,
			Nickname:  username,
			Avatar:    "",
			Age:       nil,
			Config:    map[string]any{},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
	}
}
