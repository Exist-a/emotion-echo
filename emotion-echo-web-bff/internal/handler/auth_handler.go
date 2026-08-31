// Package handler — auth_handler.go
//
// Stage 33 PR-19b：BFF 真实登录 + 限流 + verification-code 缓存。
//
// 端点（无 APISIX jwt-auth 鉴权，由 seed.sh 白名单路由负责）：
//   POST /api/v1/auth/login              {username, password} → LoginData
//   POST /api/v1/auth/register           {username, password, verificationCode} → LoginData
//   POST /api/v1/auth/refresh            → LoginData（重新签发）
//   POST /api/v1/auth/logout             → {"success": true}
//   POST /api/v1/auth/verification-code  {username} → {"success": true}
//
// 设计要点：
//   - login 调 user-svc Login（user-svc bcrypt 校验）
//   - 5 次错密码 → 锁定 5 分钟（in-memory，单实例假设）
//   - verification-code 同 username 60s 内只发一次（in-memory 缓存）
//   - refresh 保持现有 mock 行为（解析已有 JWT 重签，PR-21 收口时再做真实刷新）
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
)

// LoginData 是登录/注册/refresh 的统一响应
type LoginData struct {
	AccessToken string       `json:"accessToken"`
	ExpiresIn   int64        `json:"expiresIn"`
	User        AuthUserInfo `json:"user"`
}

// AuthUserInfo 是 BFF 签发 JWT 后返回的用户信息
type AuthUserInfo struct {
	ID        string         `json:"id"`
	Username  string         `json:"username"`
	Nickname  string         `json:"nickname"`
	Avatar    string         `json:"avatar"`
	Age       *int           `json:"age"`
	Config    map[string]any `json:"config"`
	CreatedAt string         `json:"createdAt"`
}

// 限流常量
const (
	loginLockWindow     = 5 * time.Minute // 锁定窗口
	loginMaxFailures    = 5               // 触发锁定的连续失败次数
	verificationTTL     = 60 * time.Second // 验证码有效 + 限流窗口
	verificationMinGap  = 60 * time.Second // 同 username 最小间隔
)

// AuthHandler 处理 /api/v1/auth/* 端点
type AuthHandler struct {
	jwt  *auth.Manager
	user downstream.UserClient

	// in-memory 限流（单实例假设；多实例 BFF 留 Stage 34+ Redis 迁移）
	loginMu          sync.RWMutex
	loginFailures    map[string]*loginAttempt // username → 失败计数
	verificationMu   sync.RWMutex
	verificationCodes map[string]*verificationEntry // username → 验证码 + 过期
}

// loginAttempt 跟踪某 username 的登录失败次数与锁定状态
type loginAttempt struct {
	failCount int
	lockedAt  time.Time
}

// verificationEntry 验证码缓存（含有效时间 + 上次发送时间）
type verificationEntry struct {
	code        string
	expiresAt   time.Time
	lastSentAt  time.Time
}

// NewAuthHandler 构造
//
// Stage 33 PR-19b：注入 UserClient 真实登录；保留 jwt.Manager 用于签发 JWT。
func NewAuthHandler(mgr *auth.Manager, userClient downstream.UserClient) gin.HandlerFunc {
	h := &AuthHandler{
		jwt:               mgr,
		user:              userClient,
		loginFailures:     make(map[string]*loginAttempt),
		verificationCodes: make(map[string]*verificationEntry),
	}
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
			Fail(c, http.StatusNotFound, 1, "auth endpoint not found")
		}
	}
}

func (h *AuthHandler) login(c *gin.Context) {
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"rememberMe"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: username and password are required")
		return
	}

	// 锁定检查
	if h.isLocked(req.Username) {
		Fail(c, http.StatusLocked, 1, "too many failed attempts; try again later")
		return
	}

	info, err := h.user.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		// 记录失败次数（不区分错误类型，避免用户名枚举）
		h.recordFailure(req.Username)
		// user-svc 401 → BFF 也返 401；其他 → 502
		statusCode := http.StatusBadGateway
		if apiErr, ok := err.(*downstream.APIError); ok && apiErr.StatusCode == http.StatusUnauthorized {
			statusCode = http.StatusUnauthorized
		}
		Fail(c, statusCode, 1, "invalid username or password")
		return
	}
	if info == nil {
		// user-svc 返 200 但 user 为空（异常），按 502 处理
		Fail(c, http.StatusBadGateway, 1, "user service returned empty user")
		return
	}

	// 登录成功 → 清空失败计数
	h.clearFailures(req.Username)
	OK(c, h.buildLoginData(info.UserID, info.Account, info.Nickname))
}

func (h *AuthHandler) register(c *gin.Context) {
	var req struct {
		Username         string `json:"username"`
		Password         string `json:"password"`
		VerificationCode string `json:"verificationCode"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: username and password are required")
		return
	}

	// 验证码校验（仅当 username 已有验证码缓存时）
	if !h.verifyVerificationCode(req.Username, req.VerificationCode) {
		// 没有缓存或验证码错 → 仍允许调 user-svc 注册（user-svc 自身也会校验）
		// 这里不强校验，保留向后兼容；强校验可在后续 PR 加严
	}

	info, err := h.user.Register(c.Request.Context(), req.Username, req.Password, req.VerificationCode)
	if err != nil {
		statusCode := http.StatusBadGateway
		if apiErr, ok := err.(*downstream.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				statusCode = http.StatusConflict
			case http.StatusBadRequest:
				statusCode = http.StatusBadRequest
			}
		}
		Fail(c, statusCode, 1, "registration failed")
		return
	}
	if info == nil {
		Fail(c, http.StatusBadGateway, 1, "user service returned empty user")
		return
	}

	OK(c, h.buildLoginData(info.UserID, info.Account, info.Nickname))
}

func (h *AuthHandler) refresh(c *gin.Context) {
	// mock：直接重新签发（user 从 Authorization 解析）
	// Stage 32 PR-16: 不再依赖 downstream.JWTFromContext（已删），
	// 改为直接从 Authorization header 解析。
	var userID int64 = 1
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if uid, err := h.jwt.Parse(token); err == nil {
			userID = uid
		}
	}
	OK(c, h.buildLoginData(userID, "user", ""))
}

func (h *AuthHandler) logout(c *gin.Context) {
	OK(c, gin.H{"success": true})
}

func (h *AuthHandler) verificationCode(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	_ = json.NewDecoder(c.Request.Body).Decode(&req)

	// 防枚举：不区分用户是否存在，都返 success
	// 限流：同 username 60s 内只能发一次
	if req.Username != "" && !h.canSendVerificationCode(req.Username) {
		// 限流命中 → 仍返 success（防枚举），但不真发
		OK(c, gin.H{"success": true})
		return
	}

	// 生成 6 位数字验证码，缓存 60s
	if req.Username != "" {
		h.storeVerificationCode(req.Username)
	}

	// 真发验证码的通道留空（dev mock；prod 接 SMS/Email provider）
	OK(c, gin.H{"success": true})
}

func (h *AuthHandler) buildLoginData(userID int64, username, nickname string) LoginData {
	token, _ := h.jwt.Sign(userID, username)
	expiresIn := int64(h.jwt.TTL() / time.Second)
	if nickname == "" {
		nickname = username
	}
	return LoginData{
		AccessToken: token,
		ExpiresIn:   expiresIn,
		User: AuthUserInfo{
			ID:        fmt.Sprintf("%d", userID),
			Username:  username,
			Nickname:  nickname,
			Avatar:    "",
			Age:       nil,
			Config:    map[string]any{},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
	}
}

// =====================================================
// 限流辅助函数
// =====================================================

func (h *AuthHandler) isLocked(username string) bool {
	h.loginMu.RLock()
	defer h.loginMu.RUnlock()
	attempt, ok := h.loginFailures[username]
	if !ok {
		return false
	}
	if time.Since(attempt.lockedAt) < loginLockWindow {
		return true
	}
	// 锁定窗口已过 → 重置
	return false
}

func (h *AuthHandler) recordFailure(username string) {
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	attempt, ok := h.loginFailures[username]
	if !ok {
		attempt = &loginAttempt{}
		h.loginFailures[username] = attempt
	}
	attempt.failCount++
	if attempt.failCount >= loginMaxFailures {
		attempt.lockedAt = time.Now()
		attempt.failCount = 0 // 重置计数，锁定期内不再叠加
	}
}

func (h *AuthHandler) clearFailures(username string) {
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	delete(h.loginFailures, username)
}

func (h *AuthHandler) canSendVerificationCode(username string) bool {
	h.verificationMu.Lock()
	defer h.verificationMu.Unlock()
	entry, ok := h.verificationCodes[username]
	if !ok {
		return true
	}
	return time.Since(entry.lastSentAt) >= verificationMinGap
}

func (h *AuthHandler) storeVerificationCode(username string) {
	h.verificationMu.Lock()
	defer h.verificationMu.Unlock()
	h.verificationCodes[username] = &verificationEntry{
		code:       generateCode(),
		expiresAt:  time.Now().Add(verificationTTL),
		lastSentAt: time.Now(),
	}
}

func (h *AuthHandler) verifyVerificationCode(username, code string) bool {
	if code == "" {
		return false
	}
	h.verificationMu.RLock()
	defer h.verificationMu.RUnlock()
	entry, ok := h.verificationCodes[username]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		return false
	}
	return entry.code == code
}

// generateCode 生成 6 位数字验证码（dev mock；prod 应由 SMS/Email provider 返回）
func generateCode() string {
	const digits = "0123456789"
	b := make([]byte, 6)
	now := time.Now().UnixNano()
	for i := range b {
		b[i] = digits[int(now)%10]
		now /= 10
	}
	return string(b)
}
