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
	"log/slog"
	"net/http"

	"emotion-echo-web-bff/internal/downstream"
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
	// E2E-17 D-03：带字符级时间戳的 TTS（per-char 等分近似）——
	// 前端 useTTSPlayer 用此通道的 phonemes 数组按 currentTime 驱动 BlendShape，
	// 实现真口型同步。详见 plan §2.A / F-127。
	r.POST("/api/v1/tts/phonemes", h.phonemes)
}

func (h *TTSHandler) synthesize(c *gin.Context) {
	var req downstream.SynthesizeSpeechReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Text == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: text is required")
		return
	}
	resp, err := h.ai.SynthesizeSpeech(session.WithRequestAuth(c), req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, resp)
}

func (h *TTSHandler) stream(c *gin.Context) {
	var req downstream.TTSStreamReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Text == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: text is required")
		return
	}
	// XTTS 直连（无鉴权）
	stream, err := h.xtts.Stream(c.Request.Context(), req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	defer stream.Close()

	c.Header("Content-Type", "audio/wav")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		slog.ErrorContext(c.Request.Context(), "tts-stream copy failed", "err", err)
	}
}

// phonemes 处理 /api/v1/tts/phonemes 请求，转发到 XTTS /tts_with_phonemes，
// 返回完整 JSON（audio base64 + phonemes 数组 + duration）。响应透传无封装 ——
// 前端 useTTSPlayer 直接消费 phonemes 驱动口型。
func (h *TTSHandler) phonemes(c *gin.Context) {
	var req downstream.TTSPhonemesReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Text == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: text is required")
		return
	}
	resp, err := h.xtts.Phonemes(session.WithRequestAuth(c), req)
	if err != nil {
		// statusFor(err) 把 gRPC / 4xx 错误转 HTTP（与 synthesize/stream 同款）；
		// FastAPI 4xx detail 文本已在 err 里（BFF 不二次包装），便于前端排查。
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, resp)
}
