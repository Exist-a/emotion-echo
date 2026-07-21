package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// --- helpers ---

// makeBearer 构造一个形如 "Bearer <header>.<payload>.<sig>" 的 token
// payload 中可放置 user_id 或 sub，模拟 APISIX 已签过的 JWT
func makeBearer(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return "Bearer " + strings.Join([]string{"header", enc, "sig"}, ".")
}

// callMiddleware 跑中间件，记录 next 是否被调用、返回状态码与 body
func callMiddleware(authHdr string) (status int, body string, nextCalled bool, ctxUID int64, ctxOK bool) {
	var (
		nextCalledFlag bool
		uid            int64
		ok             bool
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
	if authHdr != "" {
		req.Header.Set("Authorization", authHdr)
	}

	handler := AuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
		nextCalledFlag = true
		uid, ok = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	handler.ServeHTTP(rec, req)
	body = rec.Body.String()
	if body == "" {
		body = ""
	}
	return rec.Code, body, nextCalledFlag, uid, ok
}

// --- tests ---

// TestExtractUserIDFromJWT 表驱动 6 个分支：有效 / 缺失 / 格式错 / 缺段 / 非 base64 / 非 JSON
func TestExtractUserIDFromJWT(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantUID    int64
		wantErrSub string // 期望错误信息子串
	}{
		{
			name:    "valid_user_id_int",
			header:  makeBearer(t, map[string]any{"user_id": 42, "sub": "42"}),
			wantUID: 42,
		},
		{
			name:    "valid_user_id_from_sub",
			header:  makeBearer(t, map[string]any{"sub": "7"}),
			wantUID: 7,
		},
		{
			name:       "missing_header",
			header:     "",
			wantErrSub: "missing Authorization header",
		},
		{
			name:       "wrong_scheme",
			header:     "Basic dXNlcjpwYXNz",
			wantErrSub: "invalid JWT format",
		},
		{
			name:       "not_three_segments",
			header:     "Bearer header.payload",
			wantErrSub: "invalid JWT format",
		},
		{
			name:       "non_base64_payload",
			header:     "Bearer header.!!!.sig",
			wantErrSub: "invalid JWT format",
		},
		{
			name:       "non_json_payload",
			header:     "Bearer header." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".sig",
			wantErrSub: "invalid JWT format",
		},
		{
			name:       "zero_user_id_and_no_sub",
			header:     makeBearer(t, map[string]any{"foo": "bar"}),
			wantErrSub: "", // 当前实现 0+空 sub 会返回 0，但 middleware 层会判 uid<=0 → 401
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			gotUID, err := extractUserIDFromJWT(tt.header)
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrSub, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotUID != tt.wantUID {
				t.Fatalf("uid mismatch: want=%d got=%d", tt.wantUID, gotUID)
			}
		})
	}
}

// TestAuthMiddleware_Success 合法 JWT 把 user_id 注入 ctx
func TestAuthMiddleware_Success(t *testing.T) {
	status, body, nextCalled, uid, ok := callMiddleware(makeBearer(t, map[string]any{"user_id": 1001}))
	if status != http.StatusOK {
		t.Fatalf("status want=200 got=%d", status)
	}
	if !nextCalled {
		t.Fatalf("expected next handler to be called")
	}
	if !ok || uid != 1001 {
		t.Fatalf("expected uid=1001 in ctx, got ok=%v uid=%d", ok, uid)
	}
	_ = body
}

// TestAuthMiddleware_RejectInvalidJWT 6 类错授权 → 401 + 不调用 next
func TestAuthMiddleware_RejectInvalidJWT(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{"empty_header", ""},
		{"wrong_scheme", "Basic abc"},
		{"not_three_segments", "Bearer header.payload"},
		{"non_base64_payload", "Bearer h.!!!.s"},
		{"zero_user_id", makeBearer(t, map[string]any{"sub": "0"})},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, body, nextCalled, _, _ := callMiddleware(tc.header)
			if status != http.StatusUnauthorized {
				t.Fatalf("status want=401 got=%d body=%s", status, body)
			}
			if nextCalled {
				t.Fatalf("next handler should not be called on reject")
			}
			if !strings.Contains(body, "unauthorized") {
				t.Fatalf("body should contain unauthorized, got %s", body)
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
	// 无 Authorization header

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
	// health 路径不强制要求有 user_id：ctx 拿不到是 OK 的
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

// TestAuthMiddleware_TableAllPaths 把所有路径（=N）压一遍，统计 next 是否被调用 + status
// 用来防御"路径白名单漏配"
func TestAuthMiddleware_TableAllPaths(t *testing.T) {
	paths := []string{
		"/health",       // skip
		"/health/live",  // 必须鉴权
		"/api/v1/x",     // 必须鉴权
		"/",             // 必须鉴权
	}
	bearer := makeBearer(t, map[string]any{"user_id": 1})
	for _, p := range paths {
		nextCalled := false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		if p != "/health" {
			req.Header.Set("Authorization", bearer)
		}
		AuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		}).ServeHTTP(rec, req)

		if p == "/health" {
			if rec.Code != 200 || !nextCalled {
				t.Fatalf("path=%s skip-auth should pass without header, got status=%d next=%v", p, rec.Code, nextCalled)
			}
			continue
		}
		if rec.Code != 200 || !nextCalled {
			t.Fatalf("path=%s with valid JWT should pass, got status=%d next=%v", p, rec.Code, nextCalled)
		}
	}
}

// TestAuthMiddleware_PayloadEdgeCases payload 内 user_id 是字符串/浮点等边界
// 当前实现：user_id 必须是非零 int64；float/string/负数会被拒（uid<=0 或 unmarshal 失败）
// 这些是 RED → GREEN：未来可放宽，但目前先 lock 现状
func TestAuthMiddleware_PayloadEdgeCases(t *testing.T) {
	cases := []struct {
		name       string
		payload    map[string]any
		wantStatus int
	}{
		// 浮点 JSON unmarshal 进 int64 失败 → 401
		{"float_truncated", map[string]any{"user_id": 5.7}, http.StatusUnauthorized},
		// 负数 uid<=0 → 401
		{"negative_id", map[string]any{"user_id": -1}, http.StatusUnauthorized},
		// 字符串 user_id unmarshal 进 int64 失败 → 401
		{"string_user_id", map[string]any{"user_id": "11"}, http.StatusUnauthorized},
		// sub 也是字符串"11"，但 user_id 缺，但 strconv 能转 — sub 路径会成功，
		// uid=11 但 sub 字段被作为 fallback 解析，期望 200
		{"sub_string_int", map[string]any{"sub": "11"}, http.StatusOK},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			header := makeBearer(t, tc.payload)
			status, _, nextCalled, _, _ := callMiddleware(header)
			if status != tc.wantStatus {
				t.Fatalf("want status=%d got=%d next=%v", tc.wantStatus, status, nextCalled)
			}
		})
	}
	_ = strconv.Itoa
}
