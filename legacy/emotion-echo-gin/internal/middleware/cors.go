package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件
// 支持前端携带 Cookie/credentials
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// 允许的域名列表（开发环境）
		allowedOrigins := []string{
			"http://localhost:3000",   // React/Vue 开发服务器
			"http://localhost:3001",   // Nuxt 开发服务器（备用端口）
			"http://localhost:5173",   // Vite 默认端口
			"http://localhost:8081",   // React Native
			"http://127.0.0.1:3000",
			"http://127.0.0.1:3001",
			"http://127.0.0.1:5173",
		}
		
		// 检查是否允许的域名
		isAllowed := false
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				isAllowed = true
				break
			}
		}
		
		// 如果请求的 Origin 在允许列表中，设置具体的 Origin
		// 否则不设置 CORS 头（浏览器会拦截）
		if isAllowed && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
