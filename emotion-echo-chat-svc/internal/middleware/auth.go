// Package middleware 提供 chat-svc 的 HTTP 中间件（adapter）
//
// 实际鉴权逻辑在 shared/pkg/middleware/jwt_auth.go。
// 本文件保留仅为兼容 svc 内部 import 路径：6 个 logic 文件 + 2 个 test 都
// 用 `middleware.CtxUserIDKey{}` 提取 user_id，把 shared 包的类型 re-export
// 一下可以让 svc logic 保持短 import。
//
// Stage 41 PR-1：去掉了原 AuthMiddleware() 转发函数（返回已删除的 RestMiddleware）；
// chat-svc main 实际挂载的是 sharedmw.GinAuthMiddleware()，不走本适配层。
// Stage 41 PR-4（chat-svc 切换）时本文件会彻底删除，6 个 logic 文件改 import 路径。
package middleware

import (
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

// Re-export 让 svc logic 可以用 middleware.CtxUserIDKey 而不必改 import
type CtxUserIDKey = sharedmw.CtxUserIDKey
