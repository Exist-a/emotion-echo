// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件（Gin 版本）
//
// GinAuthMiddleware 是 jwt_auth.go 中 AuthMiddleware 的 Gin 适配版本。
// 逻辑：从 X-User-Id header 读取 user_id（已被 APISIX jwt-auth 验签后注入），
// 注入到 ctx。
//
// 流程：
//   浏览器 → APISIX jwt-auth 验证 token → 通过后注入 X-User-Id: <uid>
//          → svc 信任 APISIX（不再验证 signature）
//          → svc 读 X-User-Id header，转 int64，注入 ctx
package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

// GinAuthMiddleware 信任 APISIX 已验证的 JWT，从 X-User-Id header 读取 user_id
func GinAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过白名单端点（monitoring / metrics 不需要鉴权）
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		uid, ok := parseXUserID(c.GetHeader(XUserIDHeader))
		if !ok || uid <= 0 {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized: missing or invalid X-User-Id"})
			return
		}
		// 注入 user_id 到 ctx（与 rest 版本共享 CtxUserIDKey 类型）
		ctx := context.WithValue(c.Request.Context(), CtxUserIDKey{}, uid)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// parseXUserID 解析 X-User-Id header 值
func parseXUserID(h string) (int64, bool) {
	if h == "" {
		return 0, false
	}
	var uid int64
	for _, c := range h {
		if c < '0' || c > '9' {
			return 0, false
		}
		uid = uid*10 + int64(c-'0')
	}
	if uid <= 0 {
		return 0, false
	}
	return uid, true
}
