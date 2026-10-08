// Package handler — auth_handler.go
//
// Stage 33 PR-19b/21：BFF 真实登录（user-svc bcrypt 校验）+ 限流。
//
// 端点（无 APISIX jwt-auth 鉴权，由 deploy/apisix/seed.sh 白名单路由负责）：
//   POST /api/v1/auth/login              {username, password} → LoginData
//   POST /api/v1/auth/register           {username, password, verificationCode} → LoginData
//   POST /api/v1/auth/refresh            → LoginData（mock：解析已有 JWT 重签）
//   POST /api/v1/auth/logout             → {"success": true}
//   POST /api/v1/auth/verification-code  {username} → {"success": true}
//
// 设计要点：
//   - login 调 user-svc Login（user-svc bcrypt 校验真实密码，PR-19a）
//   - 5 次错密码 → 锁定（E2E-20：经 authlock.LoginLockStore，多实例用 redis 跨实例共享）
//   - verification-code 同 username 60s 内只能发一次（in-memory 缓存；防枚举。
//     E2E-20 收尾裁定：该端点是 D-01 决议下的遗留物待删 E2E-F-144，禁止 Redis 化）
//   - refresh：**必须带有效令牌**（D-46/E2E-29 M1：无令牌/过期/签名不符一律 401，
//     不再回落默认身份）；真实 refresh token（服务端吊销表）仍留待后续
//   - BFF 不再持有"独立 mock JWT"能力（PR-19b/21 收口）
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/authlock"
	"emotion-echo-web-bff/internal/config"
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

// accessTokenCookieName 是登录态 cookie 名（同时被 APISIX jwt-auth 读取：
// deploy/apisix/seed.sh 的 `jwt-auth.cookie = "access_token"`）。
const accessTokenCookieName = "access_token"

// 限流常量
//
// 登录锁定窗口 / 失败阈值 / 验证码最小间隔的**唯一事实源在 authlock 包**
// （`authlock/store.go`：loginLockWindow=15min / loginMaxFailures=5 /
// verificationMinGap=60s），由注入的 LoginLockStore 执行。
// 本包原先另有一套同名常量（loginLockWindow=5min 等），既未被引用、又与
// authlock 真值矛盾（5min vs 15min）⇒ E2E-29 遗留项 1 一并删除（防注释漂移）。
const (
	verificationTTL = 60 * time.Second // 验证码有效期（handler 侧保存 TTL；与 authlock 无冲突）
)

// AuthHandler 处理 /api/v1/auth/* 端点
type AuthHandler struct {
	jwt   *auth.Manager
	user  downstream.UserClient
	store authlock.LoginLockStore // E2E-20：登录失败计数（in-memory 或 redis，跨实例共享）
	vc    authlock.VerificationCodeStore // E2E-20 收尾拆出：验证码缓存（仅 in-memory，D-01 裁定遗留端点）
}

// NewAuthHandler 构造
//
// Stage 33 PR-19b：注入 UserClient 真实登录；保留 jwt.Manager 用于签发 JWT。
// Round 4.3 后补：注入 LoginLockStore（in-memory 或 redis），多实例用 redis。
// E2E-20 收尾：验证码缓存独立注入（VerificationCodeStore，仅 in-memory）——
// 该端点是 D-01 决议下的遗留物待删（E2E-F-144），禁止 Redis 化。
func NewAuthHandler(mgr *auth.Manager, userClient downstream.UserClient, store authlock.LoginLockStore, vcStore authlock.VerificationCodeStore) gin.HandlerFunc {
	if store == nil {
		// 安全兜底：注入 nil 应 panic 提示（main.go 装配时必须传）
		panic("authlock.LoginLockStore is required (pass authlock.NewInMemoryStore() or authlock.NewRedisStore(...))")
	}
	if vcStore == nil {
		panic("authlock.VerificationCodeStore is required (pass authlock.NewInMemoryStore())")
	}
	h := &AuthHandler{
		jwt:   mgr,
		user:  userClient,
		store: store,
		vc:    vcStore,
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
		case "reset-password":
			h.resetPassword(c)
		case "verify-security-answer":
			h.verifySecurityAnswer(c)
		case "security-questions":
			h.getSecurityQuestions(c)
		default:
			Fail(c, http.StatusNotFound, 1, "auth endpoint not found")
		}
	}
}

// setLockRetryAfter 在锁定响应上写 `Retry-After`（RFC 7231 §7.1.3：整数秒）。
//
// E2E-29 遗留项 1：前端 `useApi.ts:getRetryDelayMs` 优先读该头、缺失才指数退避
// 兜底；给出**剩余**秒数可让前端精确退避。d <= 0（未锁 / store 降级）时不写头。
func setLockRetryAfter(c *gin.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	secs := int((d + time.Second - 1) / time.Second) // 向上取整
	if secs < 1 {
		secs = 1
	}
	c.Header("Retry-After", strconv.Itoa(secs))
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

	// 锁定检查（Round 4.3 后补：跨实例共享通过 store 接口）
	if h.store.IsLocked(c.Request.Context(), req.Username) {
		setLockRetryAfter(c, h.store.RetryAfter(c.Request.Context(), req.Username))
		Fail(c, http.StatusLocked, 1, "too many failed attempts; try again later")
		return
	}

	info, err := h.user.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		// 记录失败次数（不区分错误类型，避免用户名枚举）
		triggered := h.store.RecordFailure(c.Request.Context(), req.Username)
		// 本次失败刚好触发锁定 → 一并给出 Retry-After，客户端无需再试
		if triggered {
			setLockRetryAfter(c, h.store.RetryAfter(c.Request.Context(), req.Username))
		}
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
	_ = h.store.ClearFailures(c.Request.Context(), req.Username)
	data := h.buildLoginData(info.UserID, info.Account, info.Nickname)
	h.setAccessTokenCookie(c, data.AccessToken, data.ExpiresIn)
	OK(c, data)
}

func (h *AuthHandler) register(c *gin.Context) {
	var req struct {
		Username           string                      `json:"username"`
		Password           string                      `json:"password"`
		VerificationCode   string                      `json:"verificationCode"`
		SecurityQuestions  []downstream.SecurityQuestion `json:"securityQuestions"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: username and password are required")
		return
	}

	// D-05: 密保问题暂时可选（后端回退），前端录入 UI 归 E2E-09
	// 若提供则校验格式（1~2 个，非空）
	if len(req.SecurityQuestions) > 2 {
		Fail(c, http.StatusBadRequest, 1, "validation: at most 2 security questions allowed")
		return
	}
	for _, sq := range req.SecurityQuestions {
		if sq.Question == "" || sq.Answer == "" {
			Fail(c, http.StatusBadRequest, 1, "validation: security question and answer cannot be empty")
			return
		}
	}

	// 验证码校验（仅当 username 已有验证码缓存时）
	if !h.verifyVerificationCode(req.Username, req.VerificationCode) {
		// 没有缓存或验证码错 → 仍允许调 user-svc 注册（user-svc 自身也会校验）
		// 这里不强校验，保留向后兼容；强校验可在后续 PR 加严
	}

	info, err := h.user.Register(c.Request.Context(), req.Username, req.Password, req.VerificationCode, req.SecurityQuestions)
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

	data := h.buildLoginData(info.UserID, info.Account, info.Nickname)
	h.setAccessTokenCookie(c, data.AccessToken, data.ExpiresIn)
	OK(c, data)
}

// refresh 用现有有效令牌换新令牌（D-46 / E2E-29 M1）。
//
// 不再回落默认身份：原实现 `var userID int64 = 1` 在 cookie 与 Authorization 均解析
// 失败时静默使用 user_id=1，配合 /api/v1/auth/ 前缀白名单（main.go:307-313）与
// APISIX route 113 放行 ⇒ **匿名调用即得 user_id=1 的 24h 有效 JWT**（E2E-F-201 实测，
// 该 token 经网关可读 /users/me）。现改为：无令牌 / 过期 / 签名不符 / 令牌类型不符
// 一律 401 且不下发 cookie；有效令牌仍正常续期（同一 user_id）。
//
// 前端零改动：useApi.ts:302-340 已有"刷新失败 → clearAuth() + 跳 /login"路径。
func (h *AuthHandler) refresh(c *gin.Context) {
	var tokenStr string
	// P0-R2-1: 优先从 HttpOnly cookie 读取，其次从 Authorization header
	if cookieToken, _ := c.Cookie(accessTokenCookieName); cookieToken != "" {
		tokenStr = cookieToken
	} else if authHeader := c.GetHeader("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if tokenStr == "" {
		Fail(c, http.StatusUnauthorized, 1, "unauthorized: refresh requires a token")
		return
	}
	userID, err := h.jwt.Parse(tokenStr)
	if err != nil {
		// Parse 已拒过期 / 签名不符 / 非 HMAC / UserID==0（reset token 属此类）
		Fail(c, http.StatusUnauthorized, 1, "unauthorized: invalid or expired token")
		return
	}
	data := h.buildLoginData(userID, "user", "")
	h.setAccessTokenCookie(c, data.AccessToken, data.ExpiresIn)
	OK(c, data)
}

func (h *AuthHandler) logout(c *gin.Context) {
	// P0-R2-1: 清除 HttpOnly cookie。
	// F-203（E2E-29）：清除必须与签发同属性（Path/HttpOnly/SameSite/Secure），
	// 否则浏览器可能留下同名 cookie 或删不干净。
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     accessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   config.IsProdMarked(),
		SameSite: http.SameSiteLaxMode,
	})
	OK(c, gin.H{"success": true})
}

func (h *AuthHandler) verificationCode(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	_ = json.NewDecoder(c.Request.Body).Decode(&req)

	// 防枚举：不区分用户是否存在，都返 success
	// 限流：同 username 60s 内只能发一次（Round 4.3 后补：通过 store 接口跨实例共享）
	if req.Username != "" && !h.vc.CanSendVerificationCode(c.Request.Context(), req.Username) {
		// 限流命中 → 仍返 success（防枚举），但不真发
		OK(c, gin.H{"success": true})
		return
	}

	// 生成 6 位数字验证码，缓存到 store（Round 4.3 后补）
	var code string
	if req.Username != "" {
		code = generateCode()
		_ = h.vc.SaveVerificationCode(c.Request.Context(), req.Username, code, verificationTTL)
	}

	// 真发验证码的通道留空（dev mock；prod 接 SMS/Email provider）。
	// Sprint 1 PR-4c-4（bug #3 修复）：dev 模式由 BFF_DEV_RETURN_CODE 控制
	// 是否在响应里回显验证码。prod 永远不回显（避免验证码泄漏到客户端日志/网络层）。
	resp := gin.H{"success": true}
	if code != "" && isDevReturnCodeEnabled() {
		resp["devCode"] = code
	}
	OK(c, resp)
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

// setAccessTokenCookie 设置 access_token cookie（P0-R2-1 防 XSS；F-203 补 SameSite/Secure）。
//
// F-203（E2E-29 计划期实测）：原实现用 `gin.Context.SetCookie` —— 该 API **没有 SameSite
// 形参**（只有 name/value/maxAge/path/domain/secure/httpOnly），于是注释声称的
// `SameSite=Lax` **从未生效**（实测 Set-Cookie 里既无 SameSite 也无 Secure）。
// 现改用 `http.SetCookie` 显式构造：
//   - SameSite=Lax：允许顶层导航携带（如 OAuth 回调），同时挡掉跨站子请求携带
//   - HttpOnly=true：JS 不可读，防 XSS 窃取
//   - Secure：随形态 —— prod 形态必须 true（HTTPS）；dev false（dev 走 HTTP，
//     Secure=true 会让浏览器直接拒收 cookie，把登录打坏）
func (h *AuthHandler) setAccessTokenCookie(c *gin.Context, token string, maxAge int64) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     accessTokenCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge),
		HttpOnly: true,
		Secure:   config.IsProdMarked(),
		SameSite: http.SameSiteLaxMode,
	})
}

// =====================================================
// 限流辅助函数（Round 4.3 后补：委托给 store 接口）
// =====================================================
//
// 以下方法已迁移到 LoginLockStore 接口，由 store 字段持有实现：
//   - isLocked → h.store.IsLocked(ctx, username)
//   - recordFailure → h.store.RecordFailure(ctx, username)
//   - clearFailures → h.store.ClearFailures(ctx, username)
//   - canSendVerificationCode → h.vc.CanSendVerificationCode(ctx, username)
//   - storeVerificationCode → h.vc.SaveVerificationCode(ctx, username, code, ttl)
//   - verifyVerificationCode → h.vc.GetVerificationCode(ctx, username)
//
// 登录锁定 in-memory 与 redis 两种实现见 authlock 子包；调用方通过
// main.go 装配时根据 LOGIN_LOCK_BACKEND=inmemory|redis env 选择。
// 验证码缓存 E2E-20 收尾拆出：仅 in-memory（D-01 裁定遗留端点，见 authlock store.go）。

// verifyVerificationCode Round 4.3 后补：从 store 取验证码并匹配。
// 行为与原方法一致：不存在/过期/不匹配都返 false。
func (h *AuthHandler) verifyVerificationCode(username, code string) bool {
	if code == "" {
		return false
	}
	saved, _ := h.vc.GetVerificationCode(h.contextForStore(), username)
	if saved == "" {
		return false
	}
	return saved == code
}

// contextForStore Round 4.3 后补：store 调用 ctx 占位（auth_handler 路径无独立 ctx）。
//
// 真实使用场景中应从 c.Request.Context() 传入；这里给 nil ctx
// 表示"用 Background 立即执行"（适用于代码 verify 这种同步短操作）。
// 若 store 要求 ctx，可改为 context.Background()。
func (h *AuthHandler) contextForStore() (ctx context.Context) {
	return context.Background()
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

// Sprint 1 PR-4c-3: resetPassword 处理 POST /api/v1/auth/reset-password
// E2E-07: 改用 resetToken 替代 verificationCode（密保验证后签发的短期 token）
// 流程：
//   1. 解析 body {resetToken, newPassword}
//   2. 校验 resetToken（JWT，5 分钟有效）
//   3. 调 user-svc /api/v1/users/reset-password（user-svc bcrypt 写库）
func (h *AuthHandler) resetPassword(c *gin.Context) {
	var req struct {
		ResetToken  string `json:"resetToken"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	if req.ResetToken == "" || req.NewPassword == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: resetToken and newPassword are required")
		return
	}
	if len(req.NewPassword) < 6 {
		Fail(c, http.StatusBadRequest, 1, "validation: newPassword must be >= 6 chars")
		return
	}
	// E2E-07: 校验 resetToken（JWT，5 分钟有效）
	username, err := h.jwt.ParseResetToken(req.ResetToken)
	if err != nil {
		Fail(c, http.StatusUnauthorized, 1, "invalid or expired reset token")
		return
	}
	// 调 user-svc（verificationCode 传 "reset-token-verified" 以满足 user-svc 接口要求）
	_, err = h.user.ResetPassword(c.Request.Context(), downstream.ResetPasswordReq{
		Username:         username,
		VerificationCode: "reset-token-verified",
		NewPassword:      req.NewPassword,
	})
	if err != nil {
		// user-svc 合并返 ErrInvalidCredentials（防用户名枚举）
		// 这里直接 500（user-svc 连接错）或 401（业务错）
		if isConnectionErr(err) {
			Fail(c, http.StatusServiceUnavailable, 1, "user-svc unavailable")
			return
		}
		Fail(c, http.StatusInternalServerError, 1, "reset password: "+err.Error())
		return
	}
	OK(c, gin.H{"success": true})
}

// verifySecurityAnswer 处理 POST /api/v1/auth/verify-security-answer（E2E-06，供 D-01=C 找回密码）
func (h *AuthHandler) verifySecurityAnswer(c *gin.Context) {
	var req struct {
		Username      string `json:"username"`
		QuestionOrder int    `json:"questionOrder"`
		Answer        string `json:"answer"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	if req.Username == "" || req.Answer == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: username and answer are required")
		return
	}
	if req.QuestionOrder < 1 || req.QuestionOrder > 2 {
		Fail(c, http.StatusBadRequest, 1, "validation: questionOrder must be 1 or 2")
		return
	}

	// R-01 #1 修复：不再用 Login(username, "dummy") 探测用户存在性
	// 直接用 user-svc 的 VerifySecurityAnswerByUsername 端点
	// user-svc 会自行处理用户不存在的情况（返回 ErrNotFound → 401 防枚举）
	err := h.user.VerifySecurityAnswerByUsername(c.Request.Context(), req.Username, req.QuestionOrder, req.Answer)
	if err != nil {
		// 防枚举：不区分"用户不存在"和"答案错误"，统一返回 401
		Fail(c, http.StatusUnauthorized, 1, "security answer verification failed")
		return
	}
	// E2E-07: 签发短期 reset token（5 分钟有效），前端传给 reset-password
	resetToken, err := h.jwt.SignResetToken(req.Username)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, "failed to sign reset token")
		return
	}
	OK(c, gin.H{"success": true, "resetToken": resetToken})
}

// E2E-07: getSecurityQuestions POST /api/v1/auth/security-questions
// 返回用户的密保问题列表（不含答案）。防枚举：用户不存在返回空列表。
func (h *AuthHandler) getSecurityQuestions(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" {
		Fail(c, http.StatusBadRequest, 1, "username is required")
		return
	}
	questions, err := h.user.GetSecurityQuestionsByUsername(c.Request.Context(), req.Username)
	if err != nil {
		// 内部错误（非防枚举）
		Fail(c, http.StatusInternalServerError, 1, "internal error")
		return
	}
	OK(c, gin.H{"questions": questions})
}

// isDevReturnCodeEnabled Sprint 1 PR-4c-4：dev 模式由 BFF_DEV_RETURN_CODE
// 控制是否在响应里回显验证码。prod 永远不回显。
func isDevReturnCodeEnabled() bool {
	v := os.Getenv("BFF_DEV_RETURN_CODE")
	return v == "1" || v == "true"
}
