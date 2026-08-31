// Package handler — emotion_query_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.52: emotion query handler（BFF → ai-svc gRPC）
//
// 端点：
//   GET /api/v1/emotion/message/:messageId        → {"emotion": Emotion}
//   GET /api/v1/emotion/conversation/:conversationId → {"emotions": [Emotion], "total": n}
package handler

import (
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
)

// EmotionQueryHandler 处理 /api/v1/emotion/* 端点
type EmotionQueryHandler struct {
	query downstream.EmotionQueryClient
}

// NewEmotionQueryHandler 构造
func NewEmotionQueryHandler(query downstream.EmotionQueryClient) *EmotionQueryHandler {
	return &EmotionQueryHandler{query: query}
}

// Register 注册路由
func (h *EmotionQueryHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/emotion/message/:messageId", h.byMessage)
	r.GET("/api/v1/emotion/conversation/:conversationId", h.byConversation)
}

func (h *EmotionQueryHandler) byMessage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("messageId"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid message id")
		return
	}
	e, err := h.query.ByMessage(c.Request.Context(), id)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"emotion": e})
}

func (h *EmotionQueryHandler) byConversation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("conversationId"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid conversation id")
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, total, err := h.query.ByConversation(c.Request.Context(), id, limit)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"emotions": items, "total": total})
}
