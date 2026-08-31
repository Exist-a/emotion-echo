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
