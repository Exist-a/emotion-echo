package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/repository"
)

// RateLimit 限流中间件
func RateLimit(redisRepo *repository.RedisRepository, rate, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 使用 IP + 完整路径（包含查询参数）作为限流 key
		key := fmt.Sprintf("ratelimit:%s:%s", c.ClientIP(), c.Request.URL.RequestURI())

		allowed, err := redisRepo.AllowRequest(c.Request.Context(), key, rate, burst)
		if err != nil {
			// 限流服务异常，拒绝请求（fail-closed）
			c.Header("Retry-After", "60")
			response.ErrorWithCode(c, errors.ErrTooManyRequests, "rate limiter unavailable")
			c.Abort()
			return
		}

		if !allowed {
			c.Header("Retry-After", "60")
			response.ErrorWithCode(c, errors.ErrTooManyRequests)
			c.Abort()
			return
		}

		c.Next()
	}
}
