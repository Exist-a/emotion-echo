package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// callMiddleware 跑中间件，记录 next 是否被调用、返回状态码与 body
func callMiddleware(headers map[string]string) (status int, body string, nextCalled bool, ctxUID int64, ctxOK bool) {
	var (
		nextCalledFlag bool
		uid            int64
		ok             bool
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	handler := AuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
		nextCalledFlag = true
		uid, ok = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), nextCalledFlag, uid, ok
}

// --- Stage 32 PR-16 RED → GREEN: 切到 X-User-Id 模式 ---

// TestAuthMiddleware_ReadsXUserIdHeader 仅带 X-User-Id → 200 + ctx uid=1001
// 当前实现只读 Authorization → 此测试必失败（RED）
func TestAuthMiddleware_ReadsXUserIdHeader(t *testing.T) {
	status, body, nextCalled, uid, ok := callMiddleware(map[string]string{"X-User-Id": "1001"})
	if status != http.StatusOK {
		t.Fatalf("status want=200 got=%d body=%s", status, body)
	}
	if !nextCalled {
		t.Fatalf("expected next handler to be called")
	}
	if !ok || uid != 1001 {
		t.Fatalf("expected uid=1001 in ctx, got ok=%v uid=%d", ok, uid)
	}
}

// TestAuthMiddleware_RejectsWhenXUserIdMissing 无 X-User-Id → 401
func TestAuthMiddleware_RejectsWhenXUserIdMissing(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"no_headers", nil},
		{"empty_x_user_id", map[string]string{"X-User-Id": ""}},
		{"non_numeric_x_user_id", map[string]string{"X-User-Id": "abc"}},
		{"zero_x_user_id", map[string]string{"X-User-Id": "0"}},
		{"negative_x_user_id", map[string]string{"X-User-Id": "-1"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, body, nextCalled, _, _ := callMiddleware(tc.headers)
			if status != http.StatusUnauthorized {
				t.Fatalf("want=401 got=%d body=%s next=%v", status, body, nextCalled)
			}
			if nextCalled {
				t.Fatalf("next should NOT be called on reject")
			}
			if !contains(body, "unauthorized") {
				t.Fatalf("body should contain 'unauthorized', got %s", body)
			}
		})
	}
}

// TestAuthMiddleware_SkipsHealth /health 应跳过鉴权
func TestAuthMiddleware_SkipsHealth(t *testing.T) {
	var nextCalled bool
	var uid int64
	var ok bool
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// 无 X-User-Id header

	handler := AuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		uid, ok = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health: want=200 got=%d", rec.Code)
	}
	if !nextCalled {
		t.Fatalf("health: next should be called")
	}
	// health 路径 ctx 无 user_id 是 OK 的
	_ = uid
	_ = ok
}

// TestUserIDFromContext 显式注入/未注入 ctx 的两路径
func TestUserIDFromContext(t *testing.T) {
	if uid, ok := UserIDFromContext(context.Background()); ok || uid != 0 {
		t.Fatalf("background ctx should yield zero value, got uid=%d ok=%v", uid, ok)
	}
	ctx := context.WithValue(context.Background(), CtxUserIDKey{}, int64(99))
	if uid, ok := UserIDFromContext(ctx); !ok || uid != 99 {
		t.Fatalf("want uid=99 ok=true, got uid=%d ok=%v", uid, ok)
	}
}

// TestAuthMiddleware_TableAllPaths 把所有路径压一遍
func TestAuthMiddleware_TableAllPaths(t *testing.T) {
	paths := []struct {
		path     string
		wantPass bool // true=期望 200, false=期望 401
	}{
		{"/health", true},         // skip
		{"/health/live", false},   // 必须鉴权
		{"/api/v1/x", false},      // 必须鉴权
		{"/", false},              // 必须鉴权
		{"/metrics", true},        // skip（与 gin 版一致）
	}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p.path, nil)
		// /health 与 /metrics 不带 X-User-Id；其他路径带
		if p.wantPass {
			// 不带 header
		} else {
			req.Header.Set("X-User-Id", "1")
		}
		nextCalled := false
		AuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		}).ServeHTTP(rec, req)

		if p.wantPass {
			if rec.Code != 200 || !nextCalled {
				t.Fatalf("path=%s skip-auth should pass, got status=%d next=%v", p.path, rec.Code, nextCalled)
			}
		} else {
			if rec.Code != 200 || !nextCalled {
				t.Fatalf("path=%s with X-User-Id=1 should pass, got status=%d next=%v", p.path, rec.Code, nextCalled)
			}
		}
	}
}

// contains substring 检查（避免引入 strings import 的不必要耦合）
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 避免 unused import 警告
var _ = strconv.Itoa
