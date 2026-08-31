// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件
//
// AuthMiddleware 从 X-User-Id header 解析 user_id（已被 APISIX jwt-auth 验签后注入），
// 注入到 ctx。
//
// 流程：
//   浏览器 → APISIX jwt-auth 验证 token → 通过后注入 X-User-Id: <uid>
//          → svc 信任 APISIX（不再验证 signature）
//          → svc 读 X-User-Id header，转 int64，注入 ctx
//
// 这样 svc 端不需要共享 JWT secret，符合"边界信任"原则。
//
// 决策 12（ADR-2026-09）：BFF 信任 APISIX 注入的 X-User-Id，不再持有 JWT secret 验证责任。
// Stage 33 PR-21 收口：原"信任 APISIX 已验过" 字样移除，明确信任边界。
package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/zeromicro/go-zero/rest"
)

// RestMiddleware 是 go-zero REST 框架的中间件类型别名
type RestMiddleware = rest.Middleware

// CtxUserIDKey 是 context 中 user id 的 key
type CtxUserIDKey struct{}

// XUserIDHeader 是 APISIX 注入的 user id header 名
const XUserIDHeader = "X-User-Id"

// AuthMiddleware 信任 APISIX 已验证的 JWT，从 X-User-Id header 读取 user_id
func AuthMiddleware() rest.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 跳过白名单端点（monitoring / metrics 不需要鉴权）
			path := r.URL.Path
			if path == "/health" || path == "/metrics" {
				next(w, r)
				return
			}

			h := r.Header.Get(XUserIDHeader)
			uid, err := strconv.ParseInt(h, 10, 64)
			if err != nil || uid <= 0 {
				http.Error(w, `{"error":"unauthorized: missing or invalid X-User-Id"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), CtxUserIDKey{}, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// UserIDFromContext 从 context 取出 user_id
func UserIDFromContext(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(CtxUserIDKey{}).(int64)
	return uid, ok
}
