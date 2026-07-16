// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件（Gin 版本）
//
// GinAuthMiddleware 是 jwt_auth.go 中 AuthMiddleware 的 Gin 适配版本。
// 逻辑：从 Authorization 头解析 JWT（已被 APISIX jwt-auth 验过），
// 提取 user_id claim，注入到 ctx。
//
// 流程：
//   浏览器 → APISIX jwt-auth 验证 token → 通过后透传到 svc
//          → svc 信任 APISIX（不再次验证 signature）
//          → svc base64 解码 JWT payload，取 user_id claim
package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

// GinAuthMiddleware 信任 APISIX 已验证的 JWT，从 Authorization 头解析 user_id
// 与 rest 框架版的区别：返回 gin.HandlerFunc 而不是 rest.Middleware
func GinAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过白名单端点（monitoring / metrics 不需要鉴权）
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		uid, err := extractUserIDFromJWT(c.GetHeader("Authorization"))
		if err != nil || uid <= 0 {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized: invalid or missing JWT"})
			return
		}
		// 注入 user_id 到 ctx（与 rest 版本共享 CtxUserIDKey 类型）
		ctx := context.WithValue(c.Request.Context(), CtxUserIDKey{}, uid)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}