// Package handler — resp.go
//
// 统一响应包装：前端 useApi 期望 ApiResponse<T> = {code, message, data}，
// code === 0 表示成功。BFF 所有 handler 用 OK/Fail 包装，避免裸 JSON 被
// useApi 判为业务错误（data.code !== 0 抛错）。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OK 成功响应（code=0）
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

// Fail 错误响应（code != 0，message 给前端）
func Fail(c *gin.Context, status, code int, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "data": nil})
}
