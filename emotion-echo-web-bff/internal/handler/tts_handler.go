// Package handler — tts_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.55-57: tts handler
//
// 端点：
//   POST /api/v1/tts/synthesize  {text, language, speed} → ai-svc → {audio(base64), sampleRate, ...}
//   POST /api/v1/tts/stream      {text, language, speed} → XTTS 直连 → raw WAV 流式转发
package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/logging"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// TTSHandler 处理 /api/v1/tts/* 端点
type TTSHandler struct {
	ai   downstream.AIClient
	xtts downstream.XTTSClient
}

// NewTTSHandler 构造（ai 与 xtts 都可选注入）
func NewTTSHandler(ai downstream.AIClient, xtts downstream.XTTSClient) *TTSHandler {
	return &TTSHandler{ai: ai, xtts: xtts}
}

// Register 注册路由
func (h *TTSHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/tts/synthesize", h.synthesize)
	r.POST("/api/v1/tts/stream", h.stream)
}

func (h *TTSHandler) synthesize(c *gin.Context) {
	var req downstream.SynthesizeSpeechReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: text is required"})
		return
	}
	resp, err := h.ai.SynthesizeSpeech(session.WithRequestAuth(c), req)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TTSHandler) stream(c *gin.Context) {
	var req downstream.TTSStreamReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: text is required"})
		return
	}
	// XTTS 直连（无鉴权）
	stream, err := h.xtts.Stream(c.Request.Context(), req)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	defer stream.Close()

	c.Header("Content-Type", "audio/wav")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		logging.Errorf(err, "[tts-stream] copy failed")
	}
}
