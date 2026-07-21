package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ginCall 调用 GinAuthMiddleware 并返回 status + next 是否被调用 + 注入的 ctxUserID
func ginCall(t *testing.T, authHeader string) (status int, body string, nextCalled bool, uid int64, uidOK bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/anything", GinAuthMiddleware(), func(c *gin.Context) {
		nextCalled = true
		if v, ok := c.Get("__uid__"); ok {
			// middle 层注入了 uid 到 gin context；我们用 UserIDFromContext 兼容两种风格
			_ = v
		}
		uid, uidOK = UserIDFromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"ok": true, "uid": uid})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), nextCalled, uid, uidOK
}

func ginCallHealth(t *testing.T, path string) (status int, nextCalled bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(path, GinAuthMiddleware(), func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(rec, req)
	return rec.Code, nextCalled
}

func makeBearerPayload(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return "Bearer " + strings.Join([]string{"header", enc, "sig"}, ".")
}

// TestGinAuthMiddleware_Success 合法 JWT 把 user_id 注入 ctx
func TestGinAuthMiddleware_Success(t *testing.T) {
	hdr := makeBearerPayload(t, map[string]any{"user_id": 77})
	status, _, nextCalled, uid, ok := ginCall(t, hdr)
	if status != http.StatusOK {
		t.Fatalf("status want=200 got=%d", status)
	}
	if !nextCalled {
		t.Fatalf("next handler should run on valid JWT")
	}
	if !ok || uid != 77 {
		t.Fatalf("ctx uid want=77 ok=true, got uid=%d ok=%v", uid, ok)
	}
}

// TestGinAuthMiddleware_Reject 5 类拒绝路径
func TestGinAuthMiddleware_Reject(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{"empty", ""},
		{"wrong_scheme", "Basic dXNlcjpwYXNz"},
		{"not_three_segments", "Bearer header.payload"},
		{"non_base64", "Bearer h.!!!.s"},
		{"zero_user_id", makeBearerPayload(t, map[string]any{"sub": "0"})},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, body, nextCalled, _, _ := ginCall(t, tc.header)
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
		status, next := ginCallHealth(t, p)
		if status != http.StatusOK {
			t.Fatalf("%s: status want=200 got=%d", p, status)
		}
		if !next {
			t.Fatalf("%s: next should be called (whitelist)", p)
		}
	}
}

// TestGinAuthMiddleware_OtherPathsAuthRequired 非白名单路径无 header 时应被拒
func TestGinAuthMiddleware_OtherPathsAuthRequired(t *testing.T) {
	status, next := ginCallHealth(t, "/api/v1/foo")
	if status != http.StatusUnauthorized {
		t.Fatalf("non-whitelisted path with no header: want=401 got=%d", status)
	}
	if next {
		t.Fatalf("non-whitelisted path should not invoke next handler")
	}
}

// TestGinAuthMiddleware_AbortSequence 多中间件链：rejection 后业务 handler 不应被调用
func TestGinAuthMiddleware_AbortSequence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/x", GinAuthMiddleware(), func(c *gin.Context) {
		// 不应触达
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
