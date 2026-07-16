package middleware

import (
	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
)

// Recovery 错误恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				response.ErrorWithCode(c, errors.ErrInternalServer)
				c.Abort()
			}
		}()
		c.Next()
	}
}
