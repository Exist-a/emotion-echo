// Package handler — auth_cookie_attrs_test.go
//
// E2E-29 F-203（计划期实测）：`Set-Cookie` 实际为
//   `access_token=…; Path=/; Max-Age=86400; HttpOnly`
// —— **没有 SameSite、没有 Secure**，而 `auth_handler.go` 的注释写着「SameSite=Lax」。
// 根因：`gin.Context.SetCookie` 没有 SameSite 形参（签名里只有 name/value/maxAge/path/
// domain/secure/httpOnly），注释描述的能力从未存在（AP-02 型：注释承诺未落地）。
//
// 本文件钉住"cookie 安全属性显式且与注释一致"：
//   - 恒有 HttpOnly + Path=/ + SameSite=Lax
//   - Secure 随形态：prod 形态（STARTUP_STRICT_DEPS 非空）必须 true；dev 必须 false
//     （dev 走 HTTP，Secure=true 会让浏览器拒收 cookie）
//   - 登出清除 cookie 必须与签发同属性（否则浏览器可能删不掉同名 cookie）
package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setCookieHeader 返回响应里 access_token 的 Set-Cookie 原始串。
func setCookieHeader(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == "access_token" {
			return w.Header().Get("Set-Cookie")
		}
	}
	return ""
}

func TestAuthHandler_Login_CookieHasSameSiteAndHttpOnly(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "")
	router := newAuthRouter(t, &fakeUserClient{login: &downstream.UserInfo{UserID: 99, Account: "alice"}})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"u","password":"p"}`)
	require.Equal(t, http.StatusOK, w.Code)

	raw := setCookieHeader(t, w)
	require.NotEmpty(t, raw, "登录必须下发 access_token cookie")
	assert.Contains(t, strings.ToLower(raw), "samesite=lax", "cookie 必须显式 SameSite=Lax（F-203：原实现没有）")
	assert.Contains(t, strings.ToLower(raw), "httponly", "cookie 必须 HttpOnly")
	assert.Contains(t, strings.ToLower(raw), "path=/", "cookie 必须 Path=/")
	assert.NotContains(t, strings.ToLower(raw), "secure", "dev 形态不得带 Secure（HTTP 下浏览器会拒收）")
}

func TestAuthHandler_Login_CookieSecureInProdForm(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "postgres,kafka")
	router := newAuthRouter(t, &fakeUserClient{login: &downstream.UserInfo{UserID: 99, Account: "alice"}})

	w := postJSON(router, "/api/v1/auth/login", `{"username":"u","password":"p"}`)
	require.Equal(t, http.StatusOK, w.Code)

	raw := setCookieHeader(t, w)
	require.NotEmpty(t, raw)
	assert.Contains(t, strings.ToLower(raw), "secure", "prod 形态 cookie 必须 Secure（HTTPS 下防明文回传）")
	assert.Contains(t, strings.ToLower(raw), "samesite=lax")
}

func TestAuthHandler_Logout_CookieClearsWithSameAttributes(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "")
	router := newAuthRouter(t, &fakeUserClient{login: &downstream.UserInfo{UserID: 99, Account: "alice"}})

	w := postJSON(router, "/api/v1/auth/logout", `{}`)
	require.Equal(t, http.StatusOK, w.Code)

	raw := setCookieHeader(t, w)
	require.NotEmpty(t, raw, "登出必须下发清除 cookie")
	lower := strings.ToLower(raw)
	assert.Contains(t, lower, "samesite=lax", "清除 cookie 需与签发同属性（SameSite）")
	assert.Contains(t, lower, "path=/")
	assert.True(t,
		strings.Contains(lower, "max-age=0") || strings.Contains(lower, "max-age=-1"),
		"清除 cookie 必须立即过期，实际=%s", raw)
}

// 防回归：gin 的 SetCookie 没有 SameSite 形参 —— 一旦有人改回去，上面断言即红。
func TestAuthHandler_CookieUsesStdlibNotGinSetCookie(t *testing.T) {
	// 源码级断言：签发/清除都必须走 http.SetCookie（可带 SameSite）
	raw, err := os.ReadFile("auth_handler.go")
	require.NoError(t, err)
	src := string(raw)
	assert.Contains(t, src, "http.SetCookie", "必须用 http.SetCookie 显式构造（gin.SetCookie 无 SameSite 形参）")
	assert.NotContains(t, src, "c.SetCookie(", "不得再用 gin.Context.SetCookie（无 SameSite 形参）")
}
