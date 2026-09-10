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
//
// Stage 41 PR-1：本文件去除 go-zero/rest 依赖，自定义 Middleware 类型；
// 原 RestMiddleware 别名已删除（chat-svc/internal/middleware/auth.go 是死代码，
// PR-1 同步删除该适配层）。
package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/emotion-echo/shared/pkg/ctxkey"
)

// Middleware 是 HTTP 中间件类型，等价 go-zero rest.Middleware 的契约。
//
// 替代方案：直接用 `func(http.HandlerFunc) http.HandlerFunc`，无需任何外部依赖。
type Middleware = func(http.HandlerFunc) http.HandlerFunc

// CtxUserIDKey 是 context 中 user id 的 key（Sprint C 重构）。
//
// 历史：原定义为独立 struct{}（jwt_auth.go:33），grpcinterceptor 又有自己的
// CtxUserIDKeyType{}，两个类型不一致导致 gRPC 拦截器注入的 user_id 与 svc logic
// 从 middleware.CtxUserIDKey 读永远 miss（决策 18 #26，Stage 63 端到端验证发现）。
//
// 改类型别名后：middleware 与 grpcinterceptor 都指向 ctxkey.UserID，编译期完全等同，
// 跨包读写同一 ctx value。现有调用 ctx.Value(CtxUserIDKey{}) 无须改（别名透明）。
type CtxUserIDKey = ctxkey.UserID

// XUserIDHeader 是 APISIX 注入的 user id header 名
const XUserIDHeader = "X-User-Id"

// AuthMiddleware 信任 APISIX 已验证的 JWT，从 X-User-Id header 读取 user_id
func AuthMiddleware() Middleware {
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
