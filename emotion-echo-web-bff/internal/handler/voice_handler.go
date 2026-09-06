// Package handler — voice_handler.go
//
// Sprint 1 PR-4c-1: 语音上传 + 转录 + 情绪分析
//
// 行为契约：
//   - POST /api/v1/voice/upload 接 multipart (conversationId, file)
//   - 调 ai-svc MultiModalAnalyze(kind=audio, file=audio bytes, fileName=recording.webm)
//   - 返回 {messageId, transcript, emotion, audioUrl}
//   - ai-svc 不可达时返 503 (语义：服务暂时不可用，区别于 500 内部错误)
//   - 缺 file 字段时返 400 + ai 不被调用
//
// Sprint 1 plan §PR-4c-1
package handler

import (
	"net/http"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// VoiceHandler 语音上传（multipart → ai-svc multimodal kind=audio）
type VoiceHandler struct {
	ai downstream.AIClient
}

// NewVoiceHandler 工厂（依赖 AIClient）
func NewVoiceHandler(ai downstream.AIClient) *VoiceHandler {
	return &VoiceHandler{ai: ai}
}

// Register 注册路由：POST /api/v1/voice/upload
func (h *VoiceHandler) Register(r *gin.Engine) {
	r.POST("/api/v1/voice/upload", h.upload)
}

// upload 处理 multipart 上传：conversationId + file → ai-svc → JSON 响应
func (h *VoiceHandler) upload(c *gin.Context) {
	// multipart 解析（gin 自带 c.Request.ParseMultipartForm；上限 10MB 留给 ai-svc 自校）
	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "invalid multipart: " + err.Error()})
		return
	}

	// file 必填
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "file 字段必填"})
		return
	}

	// 打开文件流（reader 必须 close）
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "open file: " + err.Error()})
		return
	}
	defer file.Close()

	// conversationId 选填（前端 useVoiceRecorder 总会传，但 BFF 不强制）
	conversationID := c.PostForm("conversationId")

	// 调 ai-svc multimodal kind=audio
	resp, err := h.ai.MultiModalAnalyze(c.Request.Context(), downstream.MultiModalAnalyzeReq{
		Kind:     "audio",
		File:     file,
		FileName: fileHeader.Filename,
	})
	if err != nil {
		// ai-svc 不可达 → 503 而非 500（前端可区分）
		if isConnectionErr(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code": 1,
				"message": "ai service unavailable",
				"detail": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":         0,
		"message":      "ok",
		"messageId":    uuid.NewString(), // 临时 ID；后续 chat-svc 持久化后替换
		"transcript":   resp.Transcript,
		"emotion":      resp.Emotion,
		"audioUrl":     "", // 当前 BFF 不存音频；后续 PR 加 MinIO 音频上传
		"conversationId": conversationID,
	})
}

// isConnectionErr 简单判断网络/连接类错误（ai-svc 不可达）
func isConnectionErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, hint := range []string{"connection refused", "no such host", "timeout", "dial"} {
		if contains(msg, hint) {
			return true
		}
	}
	return false
}

// contains 简单字符串包含（避免 import strings）
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}