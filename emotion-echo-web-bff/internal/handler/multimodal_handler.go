// Package handler — multimodal_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.53-54: multimodal handler（BFF → ai-svc）
//
// 端点：POST /api/v1/multimodal/analyze（multipart/form-data）
//   kind: text|image|audio（必填）
//   file: 二进制（kind≠text 时必填）
//   text: 可选
//
// 透传：multipart 原样转发到 AIClient.MultiModalAnalyze → ai-svc。
package handler

import (
	"net/http"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// MultimodalHandler 处理 /api/v1/multimodal/* 端点
type MultimodalHandler struct {
	ai downstream.AIClient
}

// NewMultimodalHandler 构造
func NewMultimodalHandler(ai downstream.AIClient) *MultimodalHandler {
	return &MultimodalHandler{ai: ai}
}

// Register 注册路由
func (h *MultimodalHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/multimodal/analyze", h.analyze)
}

func (h *MultimodalHandler) analyze(c *gin.Context) {
	kind := c.PostForm("kind")
	if kind == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: kind is required (text|image|audio)")
		return
	}

	req := downstream.MultiModalAnalyzeReq{
		Kind: kind,
		Text: c.PostForm("text"),
	}
	if kind != "text" {
		fileHeader, err := c.FormFile("file")
		if err != nil {
			Fail(c, http.StatusBadRequest, 1, "validation: file is required for kind=" + kind)
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			Fail(c, http.StatusBadRequest, 1, "cannot open uploaded file")
			return
		}
		defer file.Close()
		req.File = file
		req.FileName = fileHeader.Filename
	}

	resp, err := h.ai.MultiModalAnalyze(session.WithRequestAuth(c), req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, resp)
}
