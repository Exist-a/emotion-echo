// Package middleware 提供 chat-svc 的 HTTP 中间件（adapter）
//
// 实际鉴权逻辑在 shared/pkg/middleware/jwt_auth.go。
// 本文件保留仅为兼容 svc 内部 import 路径，避免破坏 logic 层的依赖。
package middleware

import (
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

// Re-export 让 svc logic 可以用 middleware.CtxUserIDKey 而不必改 import
type CtxUserIDKey = sharedmw.CtxUserIDKey

// AuthMiddleware 信任 APISIX 已验证的 JWT，从 Authorization 头解析 user_id
func AuthMiddleware() sharedmw.RestMiddleware {
	return sharedmw.AuthMiddleware()
}