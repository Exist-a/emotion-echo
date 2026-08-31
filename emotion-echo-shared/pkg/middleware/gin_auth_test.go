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
