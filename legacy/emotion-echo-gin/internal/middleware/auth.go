package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/jwt"
	"emotion-echo-gin/internal/pkg/response"
)

// JWTAuth JWT 认证中间件
func JWTAuth(jwtInstance *jwt.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.ErrorWithCode(c, errors.ErrTokenInvalid, "authorization header required")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.ErrorWithCode(c, errors.ErrTokenInvalid, "invalid authorization format")
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := jwtInstance.ParseToken(tokenString)
		if err != nil {
			if jwt.IsTokenExpired(err) {
				response.ErrorWithCode(c, errors.ErrTokenExpired)
			} else {
				response.ErrorWithCode(c, errors.ErrTokenInvalid)
			}
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("userId", claims.UserID)
		c.Next()
	}
}
