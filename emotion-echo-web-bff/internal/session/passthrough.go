// Package session — passthrough.go
//
// Stage 32 PR-16: 鉴权语义从"透传 JWT"改为"透传 user_id"。
//
// 职责：
//   - 从 gin.Request 提取 X-User-Id header（APISIX 注入）→ 存入 ctx
//   - handler 层通过 WithRequestAuth 包装；下游 client 通过 downstream.UserIDFromContext 读取
//   - 与 shared GinAuthMiddleware 的关系：shared 中间件已经从 ctx 注入 CtxUserIDKey，
//     本函数只是把"再透传给下游 client"的责任封装成同名 API（handler 调用面零改动）
//
// 与 downstream.WithUserID 的关系：
//   gin.Request → WithRequestAuth(ctx) → downstream.UserIDFromContext(ctx) → 下游请求
package session

import (
	"context"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

// XUserIDHeader 是 APISIX 注入的 user id header 名
const XUserIDHeader = sharedmw.XUserIDHeader

// WithRequestAuth 从 gin.Request 提取 X-User-Id 头存入 ctx。
//
// Stage 32 PR-16 之前：从 Authorization 提取 JWT token 注入 ctx（downstream.WithJWT）。
// Stage 32 PR-16 之后：从 X-User-Id 提取 user_id 注入 ctx（downstream.WithUserID）。
// Handler 层调用面不变；下游 client 改为读 user_id 并设 X-User-Id header。
//
// 若请求无 X-User-Id（如 /health），返回原 ctx（下游 client 不注入 header）。
func WithRequestAuth(c *gin.Context) context.Context {
	header := c.GetHeader(XUserIDHeader)
	if header == "" {
		return c.Request.Context()
	}
	uid, err := strconv.ParseInt(header, 10, 64)
	if err != nil || uid <= 0 {
		return c.Request.Context()
	}
	return downstream.WithUserID(c.Request.Context(), uid)
}

// AuthorizationFromContext 保留以兼容旧代码，但 Stage 32 PR-16 后 BFF 不再透传 Authorization。
// 返回空字符串，下游 client 不会再注入 Authorization header。
func AuthorizationFromContext(ctx context.Context) string {
	return ""
}
