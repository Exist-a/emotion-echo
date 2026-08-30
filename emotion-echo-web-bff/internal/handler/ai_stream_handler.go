// Package handler — ai_stream_handler.go
//
// Stage 30 / stage-30-web-bff.md T3.29-30: ai_stream handler（SSE 流式情绪分析）
//
// 路由：POST /api/v1/ai/stream
// 请求：{"messageId": 42}
// 响应：SSE 事件流（event: analysis → event: done）
//
// SSE headers（文档 §八风险表要求）：
//   Content-Type: text/event-stream
//   X-Accel-Buffering: no    ← 防止 nginx/APISIX 缓冲 SSE
//   Cache-Control: no-cache
//
// 数据源：EmotionQueryClient.ByMessage（ai-svc gRPC unary，见 sse/stream.go 偏差说明）。
package handler

import (
	"encoding/json"
	"net/http"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/logging"
	"emotion-echo-web-bff/internal/sse"

	"github.com/gin-gonic/gin"
)

// AIStreamHandler 是 /api/v1/ai/stream 的处理逻辑
type AIStreamHandler struct {
	query downstream.EmotionQueryClient
}

// NewAIStreamHandler 构造 handler（返回 gin.HandlerFunc）
func NewAIStreamHandler(query downstream.EmotionQueryClient) gin.HandlerFunc {
	h := &AIStreamHandler{query: query}
	return h.ServeHTTP
}

// ServeHTTP 处理 SSE 流式情绪分析
func (h *AIStreamHandler) ServeHTTP(c *gin.Context) {
	// 1. SSE headers（必须先于任何写入）
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 2. 解析请求
	var req struct {
		MessageID int64 `json:"messageId"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.MessageID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: messageId is required"})
		return
	}

	// 3. 调 ai-svc gRPC（unary）拿情绪分析
	emotion, err := h.query.ByMessage(c.Request.Context(), req.MessageID)
	if err != nil {
		logging.Errorf(err, "[ai-stream] ByMessage failed messageID=%d", req.MessageID)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream emotion query failed"})
		return
	}

	// 4. 编码 SSE 事件流（analysis → done）
	res := sse.AnalysisResult{
		MessageID:      emotion.MessageId,
		ConversationID: emotion.ConversationId,
		PrimaryEmotion: emotion.PrimaryEmotion,
		SentimentScore: emotion.SentimentScore,
		Confidence:     emotion.Confidence,
		Model:          emotion.Model,
	}
	if err := sse.StreamAnalysis(c.Writer, res); err != nil {
		logging.Errorf(err, "[ai-stream] SSE write failed")
		return // 连接已半开，无法再写 JSON
	}
}
