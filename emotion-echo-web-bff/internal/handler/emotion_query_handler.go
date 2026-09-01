// Package handler — emotion_query_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.52: emotion query handler（BFF → ai-svc gRPC）
//
// 端点：
//   GET /api/v1/emotion/message/:messageId           → {"emotion": Emotion}
//   GET /api/v1/emotion/conversation/:conversationId  → {"emotions": [Emotion], "total": n}
//   GET /api/v1/emotion/message/:messageId/fused     → {"fused": FusedEmotion}  (Stage 34)
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/codes"
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
	r.GET("/api/v1/emotion/message/:messageId/fused", h.byFusedMessage)
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

// byFusedMessage Stage 34：调 gRPC GetFusedEmotion。
//
// 错误映射：
//   - gRPC NotFound → 404
//   - gRPC Unimplemented（ai-svc 未装配 fused repo）→ 503
//   - 其他 gRPC 错误 → statusFor 映射（默认 502）
func (h *EmotionQueryHandler) byFusedMessage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("messageId"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid message id")
		return
	}
	f, err := h.query.ByFusedMessage(c.Request.Context(), id)
	if err != nil {
		// gRPC status 错误优先映射
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				Fail(c, http.StatusNotFound, 1, "fused emotion not found")
				return
			case codes.Unimplemented:
				Fail(c, http.StatusServiceUnavailable, 1, "fused emotion not available on this server")
				return
			}
		}
		// fallback：通用 statusFor
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	if f == nil {
		Fail(c, http.StatusNotFound, 1, "fused emotion not found")
		return
	}
	OK(c, gin.H{"fused": f})
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

// _ errors 防 unused 警告（reserved for future wrapping）
var _ = errors.New
