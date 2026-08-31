// Package handler — upload_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.58: upload handler（首期 502）
//
// 端点：POST /api/v1/uploads/{image,video,file}
//
// 文档 §七明确：upload 真支持留给 Stage 31（引入对象存储）。首期统一返回 502，
// 前端可感知"未实现"而不是 404。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// UploadHandler 处理 /api/v1/uploads/* 端点（首期 502）
type UploadHandler struct{}

// NewUploadHandler 构造
func NewUploadHandler() *UploadHandler {
	return &UploadHandler{}
}

// Register 注册路由
func (h *UploadHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/uploads/:kind", func(c *gin.Context) {
		Fail(c, http.StatusBadGateway, 1, "uploads not implemented (Stage 31)")
	})
}
