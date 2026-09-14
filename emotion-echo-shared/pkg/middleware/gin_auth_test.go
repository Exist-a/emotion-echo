package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ginCall 调用 GinAuthMiddleware 并返回 status + next 是否被调用 + 注入的 ctxUserID
//
// Stage 32 PR-16: headers map 替代原 Authorization header，
// 因为 X-User-Id 是单一 header（不再需要 Bearer JWT 格式）
func ginCall(t *testing.T, headers map[string]string) (status int, body string, nextCalled bool, uid int64, uidOK bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/anything", GinAuthMiddleware(), func(c *gin.Context) {
		nextCalled = true
		uid, uidOK = UserIDFromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"ok": true, "uid": uid})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), nextCalled, uid, uidOK
}

func ginCallHealth(t *testing.T, path string, headers map[string]string) (status int, nextCalled bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(path, GinAuthMiddleware(), func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(rec, req)
	return rec.Code, nextCalled
}

// TestGinAuthMiddleware_Success X-User-Id header 合法 → ctx uid 注入
func TestGinAuthMiddleware_Success(t *testing.T) {
	status, _, nextCalled, uid, ok := ginCall(t, map[string]string{XUserIDHeader: "77"})
	if status != http.StatusOK {
		t.Fatalf("status want=200 got=%d", status)
	}
	if !nextCalled {
		t.Fatalf("next handler should run with valid X-User-Id")
	}
	if !ok || uid != 77 {
		t.Fatalf("ctx uid want=77 ok=true, got uid=%d ok=%v", uid, ok)
	}
}

// TestGinAuthMiddleware_Reject 6 类拒绝路径
func TestGinAuthMiddleware_Reject(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"empty", nil},
		{"empty_x_user_id", map[string]string{XUserIDHeader: ""}},
		{"non_numeric", map[string]string{XUserIDHeader: "abc"}},
		{"zero_uid", map[string]string{XUserIDHeader: "0"}},
		{"negative_uid", map[string]string{XUserIDHeader: "-1"}},
		{"authorization_alone_not_enough", map[string]string{"Authorization": "Bearer xxx"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, body, nextCalled, _, _ := ginCall(t, tc.headers)
			if status != http.StatusUnauthorized {
				t.Fatalf("status want=401 got=%d body=%s", status, body)
			}
			if nextCalled {
				t.Fatalf("next handler should not run on reject")
			}
			if !strings.Contains(body, "unauthorized") {
				t.Fatalf("body should contain unauthorized, got %s", body)
			}
		})
	}
}

// TestGinAuthMiddleware_Whitelist /health 与 /metrics 跳过鉴权
func TestGinAuthMiddleware_Whitelist(t *testing.T) {
	for _, p := range []string{"/health", "/metrics"} {
		status, next := ginCallHealth(t, p, nil)
		if status != http.StatusOK {
			t.Fatalf("%s: status want=200 got=%d", p, status)
		}
		if !next {
			t.Fatalf("%s: next should be called (whitelist)", p)
		}
	}
}

// TestGinAuthMiddleware_OtherPathsAuthRequired 非白名单路径无 X-User-Id 时应被拒
func TestGinAuthMiddleware_OtherPathsAuthRequired(t *testing.T) {
	status, next := ginCallHealth(t, "/api/v1/foo", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("non-whitelisted path with no header: want=401 got=%d", status)
	}
	if next {
		t.Fatalf("non-whitelisted path should not invoke next handler")
	}
}

// TestGinAuthMiddleware_AbortSequence rejection 后业务 handler 不应被调用
func TestGinAuthMiddleware_AbortSequence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/x", GinAuthMiddleware(), func(c *gin.Context) {
		c.JSON(200, gin.H{"should": "not_reach"})
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "not_reach") {
		t.Fatalf("body should not contain not_reach, got %s", rec.Body.String())
	}
}

// =====================================================
// Stage 94 PR-6 §P0-7 · GinAuthMiddleware 加 APISIX IP 白名单
// =====================================================
//
// code-review-2026-09-14.md §P0-7 原文:GinAuthMiddleware 信任未签名 X-User-Id
// header + 白名单过窄(svc 端口若直连 → 任意用户身份)(1-2d)。
//
// 修复方案(§3.5 合并修复路径 A):GinAuthMiddleware 加 APISIX IP 白名单
// 选项(默认开启,k8s pod CIDR);dev 环境可通过 AUTH_REQUIRE_APISIX_IP=false
// 关闭(允许本地直连调试)。
//
// 这是 §P0-7 核心修复 —— 防止 svc 端口在容器外被攻击者直接访问:
//   1) 攻击者塞 X-User-Id: 1 → BFF 误信任 → 假冒 user 1
//   2) APISIX IP 白名单阻断:remoteAddr 不在白名单 → 401
//
// 测试策略:
//   1) RED: 新增 GinAuthMiddlewareWithOpts(options) + APISIX IP 白名单参数
//   2) RED: 默认 require APISIX IP,remoteAddr=127.0.0.1 不在白名单 → 401
//   3) RED: remoteAddr 在白名单 → 200
//   4) 字面量断言:shared/middleware/gin_auth.go 应有 APISIX 白名单相关 helper
//   5) 字面量断言:BFF TrustAPISIX=false 死分支应被删除(只保留 APISIX 路径)

// TestGinAuthMiddleware_RejectsXUserIdFromUntrustedRemoteAddr §P0-7 RED:
//
// 旧实现（§P0-7 bug）:任何 remoteAddr 只要带 X-User-Id header 即放行 → 攻击者
// 绕过 APISIX 直接 dial svc 端口塞 header 即可假冒。
//
// 新实现:GinAuthMiddlewareWithOpts 接受 APISIX IP 白名单,白名单外 remoteAddr
// 即使带 X-User-Id 也必须 401。
func TestGinAuthMiddleware_RejectsXUserIdFromUntrustedRemoteAddr(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// APISIX pod CIDR 白名单设为 10.0.0.0/8(k8s 内部),remoteAddr 127.0.0.1 不在内
	r.GET("/api/v1/x", GinAuthMiddlewareWithOpts(AuthOpts{
		RequireAPISIXIP: true,
		APISIXCIDRs:     []string{"10.0.0.0/8"},
	}), func(c *gin.Context) {
		c.JSON(200, gin.H{"should": "not_reach"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set(XUserIDHeader, "1")     // 攻击者塞 header
	req.RemoteAddr = "127.0.0.1:55555"      // 不在 APISIX 白名单
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("§P0-7 修复要求:remoteAddr=127.0.0.1 不在 APISIX IP 白名单 → 401,\n"+
			"实际 status=%d body=%s —— 若仍 200 说明 svc 仍信任未签名 X-User-Id", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "not_reach") {
		t.Error("handler should not be invoked from untrusted remote addr")
	}
}

// TestGinAuthMiddleware_AcceptsXUserIdFromAPISIXIP §P0-7 GREEN:
//
// APISIX IP 白名单内的 remoteAddr 带 X-User-Id → 200 + ctx 注入 uid。
func TestGinAuthMiddleware_AcceptsXUserIdFromAPISIXIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/x", GinAuthMiddlewareWithOpts(AuthOpts{
		RequireAPISIXIP: true,
		APISIXCIDRs:     []string{"10.0.0.0/8"},
	}), func(c *gin.Context) {
		uid, ok := UserIDFromContext(c.Request.Context())
		c.JSON(200, gin.H{"uid": uid, "ok": ok})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set(XUserIDHeader, "42")
	req.RemoteAddr = "10.1.2.3:55555" // APISIX pod IP,白名单内
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("APISIX IP 白名单内应通过, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGinAuthMiddleware_RequireAPISIXIP_DisabledForDev §P0-7 GREEN:
//
// dev 环境 AUTH_REQUIRE_APISIX_IP=false → 与旧实现等价,任何 remoteAddr 都接受 X-User-Id
// (向后兼容 Stage 32 PR-16 dev 本地调试)。
func TestGinAuthMiddleware_RequireAPISIXIP_DisabledForDev(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/x", GinAuthMiddlewareWithOpts(AuthOpts{
		RequireAPISIXIP: false, // dev mode
	}), func(c *gin.Context) {
		uid, ok := UserIDFromContext(c.Request.Context())
		c.JSON(200, gin.H{"uid": uid, "ok": ok})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set(XUserIDHeader, "99")
	req.RemoteAddr = "127.0.0.1:55555" // dev 本机
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("dev mode 应接受本地 X-User-Id, got status=%d", rec.Code)
	}
}
