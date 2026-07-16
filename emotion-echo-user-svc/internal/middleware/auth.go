// Package middleware 提供 user-svc 的 HTTP 中间件（Gin adapter）
//
// 实际鉴权逻辑在 shared/pkg/middleware/gin_auth.go。
// 本目录保留仅为兼容 logic 层的 import（middleware.CtxUserIDKey）。
package middleware

import (
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

// Re-export 让 logic 层代码不变
type CtxUserIDKey = sharedmw.CtxUserIDKey

// (Gin 风格鉴权直接用 sharedmw.GinAuthMiddleware()，无需 adapter)