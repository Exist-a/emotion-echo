// Package handler — auth_handler_test.go
//
// Stage 33 PR-19b：BFF 真实登录 + 限流的 TDD 测试。
package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apiError 是 *downstream.APIError 的 type alias 用于断言
type apiError = downstream.APIError

func newAuthRouter(t *testing.T, client downstream.UserClient) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mgr, err := auth.NewManager("test-secret", 3600)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/api/v1/auth/:action", NewAuthHandler(mgr, client))
	return router
}

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// =============================================================================
// Login tests（PR-19b）
// =============================================================================

func TestAuthHandler_Login_UserClientReturnsUser_ReturnsToken(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 42, Account: "alice", Nickname: "Alice"},
	})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"correct"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)
	assert.NotEmpty(t, data.AccessToken)
	assert.Equal(t, int64(42), mustParseInt(t, data.User.ID))
	assert.Equal(t, "alice", data.User.Username)
	assert.Equal(t, "Alice", data.User.Nickname)
}

func TestAuthHandler_Login_UserClient401_Returns401(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		loginErr: &apiError{StatusCode: http.StatusUnauthorized, Msg: "invalid"},
	})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"wrong"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Login_UserClient5xx_Returns502(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		loginErr: &apiError{StatusCode: http.StatusInternalServerError, Msg: "db down"},
	})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"any"}`)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestAuthHandler_Login_EmptyCredentials_Returns400(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"","password":""}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_Login_5Failures_LocksAccount(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		loginErr: &apiError{StatusCode: http.StatusUnauthorized, Msg: "invalid"},
	})

	// 连续 5 次错密码
	for i := 0; i < 5; i++ {
		w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"wrong"}`)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "第 %d 次失败应返 401", i+1)
	}

	// 第 6 次 → 即便 mock 返 user，也应被锁定返 423
	w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"right"}`)
	assert.Equal(t, http.StatusLocked, w.Code, "5 次失败后第 6 次必须返 423")
}

func TestAuthHandler_Login_AfterLock_5Minutes_AllowsRetry(t *testing.T) {
	// 验证锁定窗口内仍返 423：5 次失败后用同一 router 实例第 6 次
	router := newAuthRouter(t, &fakeUserClient{
		login:    &downstream.UserInfo{UserID: 42, Account: "alice"},
		loginErr: &apiError{StatusCode: http.StatusUnauthorized, Msg: "invalid"},
	})

	// 触发 5 次锁定（前 4 次返 401，第 5 次也返 401 但内部已 lock）
	for i := 0; i < 5; i++ {
		postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"wrong"}`)
	}

	// 第 6 次：fakeUserClient 切换到 success，但锁定仍在 → 应仍返 423
	w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"right"}`)
	assert.Equal(t, http.StatusLocked, w.Code, "锁定窗口内应仍返 423")
}

func TestAuthHandler_Login_TokenCanBeParsed_BackToUserID(t *testing.T) {
	mgr, _ := auth.NewManager("test-secret", 3600)
	router := gin.New()
	router.POST("/api/v1/auth/:action", NewAuthHandler(mgr, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 100, Account: "carol"},
	}))

	w := postJSON(router, "/api/v1/auth/login", `{"username":"carol","password":"x"}`)
	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)

	uid, err := mgr.Parse(data.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, int64(100), uid, "token 内 user_id 应等于响应 user.id")
}

// =============================================================================
// Register tests
// =============================================================================

func TestAuthHandler_Register_UserClientReturnsUser_ReturnsToken(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		reg: &downstream.UserInfo{UserID: 5, Account: "new", Nickname: "New"},
	})

	w := postJSON(router, "/api/v1/auth/register", `{"username":"new","password":"validpass","verificationCode":"1234"}`)
	require.Equal(t, http.StatusOK, w.Code)

	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)
	assert.NotEmpty(t, data.AccessToken)
	assert.Equal(t, int64(5), mustParseInt(t, data.User.ID))
}

func TestAuthHandler_Register_UsernameTaken_Returns409(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		regErr: &apiError{StatusCode: http.StatusConflict, Msg: "username taken"},
	})

	w := postJSON(router, "/api/v1/auth/register", `{"username":"existing","password":"validpass","verificationCode":"1234"}`)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAuthHandler_Register_EmptyFields_Returns400(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})

	w := postJSON(router, "/api/v1/auth/register", `{"username":"","password":""}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// verification-code 限流测试
// =============================================================================

func TestAuthHandler_VerificationCode_FirstRequest_ReturnsSuccess(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})

	w := postJSON(router, "/api/v1/auth/verification-code", `{"username":"alice"}`)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_VerificationCode_Within60s_ReturnsSuccess_NoResend(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})

	w1 := postJSON(router, "/api/v1/auth/verification-code", `{"username":"alice"}`)
	w2 := postJSON(router, "/api/v1/auth/verification-code", `{"username":"alice"}`)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, http.StatusOK, w2.Code)
	// 第二次仍返 success（防枚举语义），但 store 不更新 → 验证码不变
}

// =============================================================================
// Logout tests
// =============================================================================

func TestAuthHandler_Logout_ReturnsSuccess(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
}

// TestAuthHandler_Login_SetsAccessTokenCookie_P0R2_1 P0-R2-1: 登录成功后 BFF
// 必须通过 Set-Cookie: access_token=<jwt>; HttpOnly 把 token 下发给浏览器。
//
// 背景：原实现仅在 JSON body 里返 accessToken，前端 useApi.ts 把 token 同时
// 存进 localStorage + 非 HttpOnly cookie → XSS 一键登录绕过任何后端鉴权。
//
// 修复：handler.login 调 setAccessTokenCookie(c, ...) 注入 HttpOnly cookie，
// APISIX jwt-auth 通过 cookie: "access_token" 校验，浏览器 JS 不可读。
//
// 本测试钉死 4 个行为：
//   1. 响应头含 Set-Cookie，cookie 名正确
//   2. cookie 值等于 JSON body 的 accessToken
//   3. cookie 含 HttpOnly flag（防 XSS 窃取）
//   4. cookie Max-Age 等于 token 的 expiresIn 秒
func TestAuthHandler_Login_SetsAccessTokenCookie_P0R2_1(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 42, Account: "alice", Nickname: "Alice"},
	})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"correct"}`)
	require.Equal(t, http.StatusOK, w.Code)

	// 解析 JSON body 拿 token
	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)
	require.NotEmpty(t, data.AccessToken, "JSON body 仍需返 accessToken（向后兼容）")

	// 1) + 2) Set-Cookie 必须含 access_token=<token>
	cookies := w.Result().Cookies()
	require.NotEmpty(t, cookies, "登录响应必须 Set-Cookie（P0-R2-1）")
	var found *http.Cookie
	for _, ck := range cookies {
		if ck.Name == "access_token" {
			found = ck
			break
		}
	}
	require.NotNil(t, found, "Set-Cookie 必须含 access_token（APISIX jwt-auth 依赖此名）")
	assert.Equal(t, data.AccessToken, found.Value,
		"cookie 值必须等于 JSON body 的 accessToken（保证 APISIX 验证一致）")

	// 3) HttpOnly flag —— 浏览器 JS document.cookie 不可读
	assert.True(t, found.HttpOnly, "access_token cookie 必须 HttpOnly（防 XSS 窃取，P0-R2-1）")

	// 4) Max-Age —— 与 token expiresIn 对齐
	assert.Greater(t, found.MaxAge, 0, "access_token cookie Max-Age 必须 > 0")
}

// TestAuthHandler_Logout_ClearsAccessTokenCookie_P0R2_1 P0-R2-1: 登出时
// 必须清掉 HttpOnly cookie，否则旧 token 残留 → 复用旧会话。
func TestAuthHandler_Logout_ClearsAccessTokenCookie_P0R2_1(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 42, Account: "alice"},
	})
	postJSON(router, "/api/v1/auth/login", `{"username":"alice","password":"correct"}`)

	w := postJSON(router, "/api/v1/auth/logout", `{}`)
	require.Equal(t, http.StatusOK, w.Code)

	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, ck := range cookies {
		if ck.Name == "access_token" {
			found = ck
			break
		}
	}
	require.NotNil(t, found, "登出响应必须 Set-Cookie access_token=; Max-Age=-1")
	assert.Equal(t, -1, found.MaxAge, "登出 cookie Max-Age 必须 -1，立即过期")
	assert.True(t, found.HttpOnly, "登出 cookie 也必须 HttpOnly（与登录对称）")
}

// TestAuthHandler_Refresh_PrefersCookieOverHeader_P0R2_1 P0-R2-1: refresh
// 接口优先读 HttpOnly cookie 中的 token，fallback 才走 Authorization header。
// 浏览器刷新页面后 Pinia 状态丢失但 cookie 仍在，必须能继续 refresh。
func TestAuthHandler_Refresh_PrefersCookieOverHeader_P0R2_1(t *testing.T) {
	mgr, _ := auth.NewManager("test-secret", 3600)
	// 用真实 jwt 签发 cookie token
	cookieToken, err := mgr.Sign(99, "alice")
	require.NoError(t, err)

	router := newAuthRouter(t, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 99, Account: "alice"},
	})

	// 模拟浏览器：Cookie header 带 access_token，无 Authorization
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Cookie", "access_token="+cookieToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)
	uid, err := mgr.Parse(data.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, int64(99), uid, "refresh 应优先从 HttpOnly cookie 读 user_id=99")
}

// TestAuthHandler_Refresh_FallsBackToAuthorizationHeader BFF 当前保留兼容：
// 若无 cookie 才走 Authorization header（前端老版本/CLI/Postman 仍能 refresh）。
func TestAuthHandler_Refresh_FallsBackToAuthorizationHeader(t *testing.T) {
	mgr, _ := auth.NewManager("test-secret", 3600)
	headerToken, err := mgr.Sign(77, "bob")
	require.NoError(t, err)

	router := newAuthRouter(t, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 77, Account: "bob"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	// 不带 cookie
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)
	uid, err := mgr.Parse(data.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, int64(77), uid, "无 cookie 时 fallback 到 Authorization header")
}

// =============================================================================
// Refresh 测试（保持 mock 行为）
// =============================================================================

func TestAuthHandler_Refresh_ReturnsNewToken(t *testing.T) {
	mgr, _ := auth.NewManager("test-secret", 3600)
	router := gin.New()
	router.POST("/api/v1/auth/:action", NewAuthHandler(mgr, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 99, Account: "u"},
	}))

	// 先登录拿 token
	w := postJSON(router, "/api/v1/auth/login", `{"username":"u","password":"p"}`)
	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)

	// refresh 用该 token
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+data.AccessToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
	var refreshed LoginData
	decodeData(t, w2.Body.Bytes(), &refreshed)
	assert.NotEmpty(t, refreshed.AccessToken)
}

// =============================================================================
// 工具函数
// =============================================================================

func mustParseInt(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	require.NoError(t, err)
	return n
}

// 避免 unused import 警告（errors / context）
var _ = errors.New
var _ = context.Background
var _ = time.Second

// =============================================================================
// Sprint 1 PR-4c-3: resetPassword handler tests
// =============================================================================

// fakeAuth 暴露 verification-code 内部缓存给测试用
// (生产代码里 BFF 内部 state 用 sync.Mutex 保护；测试直接构造 entry)
type fakeVerificationEntry struct {
	code      string
	expiresAt time.Time
}

func TestAuthHandler_ResetPassword_InvalidVerificationCode_Returns401(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{err: nil})
	body := `{"username":"alice","verificationCode":"WRONG","newPassword":"new-password-789"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_ResetPassword_EmptyFields_Returns400(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})
	for _, body := range []string{
		`{"username":"","verificationCode":"1","newPassword":"x"}`,
		`{"username":"u","verificationCode":"","newPassword":"x"}`,
		`{"username":"u","verificationCode":"1","newPassword":""}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", body)
	}
}

func TestAuthHandler_ResetPassword_ShortPassword_Returns400(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})
	body := `{"username":"alice","verificationCode":"1","newPassword":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_ResetPassword_InvalidBody_Returns400(t *testing.T) {
	router := newAuthRouter(t, &fakeUserClient{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// Sprint 1 PR-4c-4: verification-code dev 回显（修 bug #3）
//
// 之前 verification-code 端点只把验证码存到 BFF 内部缓存，外部拿不到明文。
// commit msg（928bed2 "未做"小节）承诺"前端 console 显示"——但前端代码
// 根本没实现 console 输出，dev 模式 e2e 跑不通。
//
// RED 期望：verification-code 响应 data.success=true 时同时带回 devCode（仅 dev 模式）。
// GREEN 实现：handler 用 os.Getenv("BFF_DEV_RETURN_CODE") 控制开/关。
// =============================================================================

func TestAuthHandler_VerificationCode_DevMode_ReturnsDevCode(t *testing.T) {
	t.Setenv("BFF_DEV_RETURN_CODE", "1")
	router := newAuthRouter(t, &fakeUserClient{})

	w := postJSON(router, "/api/v1/auth/verification-code", `{"username":"e2e_dev_user"}`)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Regexp(t, `"devCode":"[0-9]{6}"`, body, "dev mode must echo the 6-digit code for e2e")
}

func TestAuthHandler_VerificationCode_ProdMode_NoDevCode(t *testing.T) {
	t.Setenv("BFF_DEV_RETURN_CODE", "")
	router := newAuthRouter(t, &fakeUserClient{})

	w := postJSON(router, "/api/v1/auth/verification-code", `{"username":"e2e_prod_user"}`)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.NotContains(t, body, "devCode", "prod mode must not echo the code")
}

// =============================================================================
// Stage 62 PR-1: login throttle helper-only tests (isLocked/recordFailure/clearFailures)
// 直接构造 *AuthHandler 测内部 state 转换，不走 HTTP —— 补 plan §二.1 缺失的
// "5 分钟后解锁" 与 "清空失败计数后重新允许" 等 helper 层断言
// =============================================================================

func newTestAuthHandler() *AuthHandler {
	return &AuthHandler{
		loginFailures:     make(map[string]*loginAttempt),
		verificationCodes: make(map[string]*verificationEntry),
	}
}

func TestAuthHandler_RecordFailure_BelowThreshold_DoesNotLock(t *testing.T) {
	h := newTestAuthHandler()
	for i := 0; i < loginMaxFailures-1; i++ {
		h.recordFailure("alice")
	}
	assert.False(t, h.isLocked("alice"), "第 %d 次失败未达阈值，不应锁", loginMaxFailures-1)
}

func TestAuthHandler_RecordFailure_AtThreshold_Locks(t *testing.T) {
	h := newTestAuthHandler()
	for i := 0; i < loginMaxFailures; i++ {
		h.recordFailure("alice")
	}
	assert.True(t, h.isLocked("alice"), "第 %d 次失败应锁定", loginMaxFailures)
}

func TestAuthHandler_RecordFailure_AfterLock_ResetsCount(t *testing.T) {
	// recordFailure 触发 lock 后，failCount 应被重置为 0（避免锁期内叠加）
	h := newTestAuthHandler()
	for i := 0; i < loginMaxFailures+2; i++ {
		h.recordFailure("alice")
	}
	h.loginMu.RLock()
	attempt := h.loginFailures["alice"]
	h.loginMu.RUnlock()
	assert.NotNil(t, attempt)
	assert.Equal(t, 0, attempt.failCount, "锁定后 failCount 应重置为 0（防锁期内叠加）")
}

func TestAuthHandler_ClearFailures_RemovesLock(t *testing.T) {
	h := newTestAuthHandler()
	for i := 0; i < loginMaxFailures; i++ {
		h.recordFailure("alice")
	}
	require.True(t, h.isLocked("alice"))
	h.clearFailures("alice")
	assert.False(t, h.isLocked("alice"), "登录成功后 clearFailures 应解锁")
	h.loginMu.RLock()
	_, exists := h.loginFailures["alice"]
	h.loginMu.RUnlock()
	assert.False(t, exists, "loginFailures map 应删该 username entry")
}

func TestAuthHandler_IsLocked_UnknownUser_ReturnsFalse(t *testing.T) {
	h := newTestAuthHandler()
	assert.False(t, h.isLocked("nobody"), "未知 username 不应在 map → 返 false")
}

func TestAuthHandler_PerUserIsolation(t *testing.T) {
	// alice 锁定不应影响 bob
	h := newTestAuthHandler()
	for i := 0; i < loginMaxFailures; i++ {
		h.recordFailure("alice")
	}
	assert.True(t, h.isLocked("alice"))
	assert.False(t, h.isLocked("bob"), "per-user 锁定应隔离")
}

func TestAuthHandler_IsLocked_AfterWindowExpires(t *testing.T) {
	// 手动构造 6 分钟前的 lockedAt → 验证 isLocked 判定窗口已过
	h := newTestAuthHandler()
	h.loginFailures["alice"] = &loginAttempt{
		failCount: 0,
		lockedAt:  time.Now().Add(-6 * time.Minute),
	}
	assert.False(t, h.isLocked("alice"), "5min 锁定窗口已过 → isLocked 应返 false")
}
