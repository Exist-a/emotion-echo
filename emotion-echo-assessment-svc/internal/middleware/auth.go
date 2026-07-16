// Package middleware 提供 assessment-svc 的 HTTP 中间件（adapter）
//
// 实际鉴权逻辑在 shared/pkg/middleware/。
// 本目录保留仅为兼容 logic 层 import（middleware.CtxUserIDKey）。
package middleware

import (
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

// Re-export 让 logic 层代码不变
type CtxUserIDKey = sharedmw.CtxUserIDKey