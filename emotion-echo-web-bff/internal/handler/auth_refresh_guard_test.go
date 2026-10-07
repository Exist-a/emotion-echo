// Package handler — auth_refresh_guard_test.go
//
// E2E-29 L1（D-46，2026-10-07 用户拍板）：/api/v1/auth/refresh 在拿不到**有效**令牌时
// 必须 401 —— 不得再回落默认身份。
//
// 背景（E2E-F-201 计划期实测）：原实现 `var userID int64 = 1` 在 cookie/header 均
// 解析失败时静默使用 user_id=1，于是匿名 `POST /api/v1/auth/refresh`（该端点同时被
// BFF 前缀白名单与 APISIX route 113 放行）返回 200 并发放 user_id=1 的 24h 有效 JWT
// ⇒ 认证绕过（实测该 token 经网关可读 /users/me 返 account=echo）。
//
// 本文件钉死四条拒绝路径（缺失/过期/签名不符/令牌类型不符）+ 一条续期路径。
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/authlock"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const refreshGuardSecret = "test-secret"

func newRefreshGuardRouter(t *testing.T) (*gin.Engine, *auth.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mgr, err := auth.NewManager(refreshGuardSecret, 3600)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/api/v1/auth/:action", NewAuthHandler(mgr, &fakeUserClient{
		login: &downstream.UserInfo{UserID: 42, Account: "u"},
	}, authlock.NewInMemoryStore(), authlock.NewInMemoryStore()))
	return router, mgr
}

// signExpired 用给定 secret 签一个 exp 已过的 access token。
// NewManager 不接受 ttl<=0（会回落 24h），故测试直接构造 claims 再签。
func signExpired(t *testing.T, secret string, userID int64) string {
	t.Helper()
	claims := auth.Claims{
		UserID:   userID,
		User:     "user",
		Key:      "user",
		Username: "u",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			Subject:   "42",
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return tok
}

func postRefresh(router *gin.Engine, cookieToken, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	if cookieToken != "" {
		req.Header.Set("Cookie", "access_token="+cookieToken)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// #1 匿名（无 cookie、无 Authorization）必须 401，且不得设置 cookie / 返回 accessToken。
func TestAuthHandler_Refresh_NoToken_Returns401AndNoCookie(t *testing.T) {
	router, _ := newRefreshGuardRouter(t)

	w := postRefresh(router, "", "")

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"匿名 refresh 必须 401（E2E-F-201：原实现回落到 user_id=1）")
	assert.Empty(t, w.Header().Get("Set-Cookie"), "401 时不得下发 access_token cookie")
	assert.NotContains(t, w.Body.String(), "accessToken", "401 时不得返回 accessToken")
}

// #2 过期 token（cookie 路径）必须 401 —— 不得因"解析失败"而回落默认身份。
func TestAuthHandler_Refresh_ExpiredTokenInCookie_Returns401(t *testing.T) {
	router, _ := newRefreshGuardRouter(t)

	w := postRefresh(router, signExpired(t, refreshGuardSecret, 42), "")

	require.Equal(t, http.StatusUnauthorized, w.Code, "过期 token 必须 401，不得续期")
	assert.Empty(t, w.Header().Get("Set-Cookie"))
}

// #3 过期 token（Authorization header 路径）同型。
func TestAuthHandler_Refresh_ExpiredTokenInHeader_Returns401(t *testing.T) {
	router, _ := newRefreshGuardRouter(t)

	w := postRefresh(router, "", "Bearer "+signExpired(t, refreshGuardSecret, 42))

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, w.Header().Get("Set-Cookie"))
}

// #4 签名不符（另一把 secret 签的 token）必须 401。
func TestAuthHandler_Refresh_WrongSignature_Returns401(t *testing.T) {
	router, mgr := newRefreshGuardRouter(t)
	foreign, err := auth.NewManager("another-secret", 3600)
	require.NoError(t, err)
	foreignToken, err := foreign.Sign(42, "u")
	require.NoError(t, err)

	w := postRefresh(router, foreignToken, "")

	require.Equal(t, http.StatusUnauthorized, w.Code, "签名不符必须 401")
	// 负向对照：本 manager 自己的有效 token 仍应 200（证明上面拦的是签名而非路径）
	ownToken, err := mgr.Sign(42, "u")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, postRefresh(router, ownToken, "").Code)
}

// #5 令牌类型隔离：reset token（密保验证用，无 UserID + Subject=reset-password）
// 不得当作 access token 续期。
func TestAuthHandler_Refresh_ResetToken_Returns401(t *testing.T) {
	router, mgr := newRefreshGuardRouter(t)
	resetToken, err := mgr.SignResetToken("alice")
	require.NoError(t, err)

	w := postRefresh(router, resetToken, "")

	require.Equal(t, http.StatusUnauthorized, w.Code, "reset token 不得当 access token 用")
}

// #6 续期路径保持可用：有效 token → 200，且新 token 解析出**同一** user_id（防"续期换身份"）。
func TestAuthHandler_Refresh_ValidToken_RenewsSameUser(t *testing.T) {
	router, mgr := newRefreshGuardRouter(t)
	valid, err := mgr.Sign(42, "u")
	require.NoError(t, err)

	w := postRefresh(router, valid, "")

	require.Equal(t, http.StatusOK, w.Code)
	var data LoginData
	decodeData(t, w.Body.Bytes(), &data)
	uid, err := mgr.Parse(data.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, int64(42), uid, "续期必须保持同一 user_id")
	assert.NotEmpty(t, w.Header().Get("Set-Cookie"), "成功续期应刷新 cookie")
}
