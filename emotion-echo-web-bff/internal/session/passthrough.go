// Package session — passthrough.go
//
// Stage 30 / stage-30-web-bff.md T3.31 REFACTOR: Session/JWT 透传 helper。
//
// 职责：
//   - 从 gin.Request 提取 Authorization header（Bearer JWT）→ 存入 ctx
//   - handler 层通过 WithRequestAuth 注入 ctx；下游 client 通过 JWTFromContext 透传
//   - user_id 读取（BFF 挂 shared GinAuthMiddleware 后注入 CtxUserIDKey）
//
// 与 downstream.WithJWT 的关系：session 包是"上游→BFF"方向（取请求头），
// downstream 是"BFF→下游"方向（发请求时注入头）。两者通过 context 衔接：
//   gin.Request → WithRequestAuth(ctx) → downstream.JWTFromContext(ctx) → 下游请求
package session

import (
	"context"
	"strings"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
)

// AuthHeader 是 Authorization header 名
const AuthHeader = "Authorization"

// WithRequestAuth 从 gin.Request 提取 Authorization 头存入 ctx。
//
// 若请求无 Authorization（如 /health），返回原 ctx（下游 client 不注入头）。
func WithRequestAuth(c *gin.Context) context.Context {
	header := c.GetHeader(AuthHeader)
	if header == "" {
		return c.Request.Context()
	}
	token := strings.TrimPrefix(header, "Bearer ")
	if token == header { // 非 Bearer 格式 → 原样传（下游决定）
		token = header
	}
	return downstream.WithJWT(c.Request.Context(), token)
}

// AuthorizationFromContext 从 ctx 取完整 Authorization 头值（"Bearer xxx"）。
// 供聚合 handler 需要把请求头原样透传时使用。
func AuthorizationFromContext(ctx context.Context) string {
	token := downstream.JWTFromContext(ctx)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}
