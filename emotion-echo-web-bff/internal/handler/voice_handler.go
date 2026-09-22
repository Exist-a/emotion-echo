// Package handler — voice_handler.go
//
// Sprint 1 PR-4c-1: 语音上传 + 转录 + 情绪分析
// D-11 (E2E-16): 音频落 MinIO（voice/ 前缀） + 返回 audioUrl，使前端 VoiceMessage
// 分支可达、气泡可回放
//
// 行为契约：
//   - POST /api/v1/voice/upload 接 multipart (conversationId, file)
//   - 调 ai-svc MultiModalAnalyze(kind=audio, file=audio bytes, fileName=recording.webm)
//   - 音频落 MinIO（voice/<conversationId>-<uuid>.webm）→ 返回 audioUrl
//   - 返回 {messageId, transcript, emotion, audioUrl}
//   - ai-svc 不可达时返 503 (语义：服务暂时不可用，区别于 500 内部错误)
//   - 缺 file 字段时返 400 + ai 不被调用
//   - storage 未配置时返 503（防止"录音看似成功但回放空白"的不可见失败）
//
// Sprint 1 plan §PR-4c-1
package handler

import (
	"fmt"
	"net/http"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// VoiceHandler 语音上传（multipart → ai-svc multimodal kind=audio + 音频落 MinIO）
type VoiceHandler struct {
	ai      downstream.AIClient
	storage storageClient // 可选：D-11 起语音音频落到 MinIO（voice/ 前缀），nil = 503
}

// NewVoiceHandler 工厂（依赖 AIClient；storage 可单独 Set）
func NewVoiceHandler(ai downstream.AIClient) *VoiceHandler {
	return &VoiceHandler{ai: ai}
}

// WithStorage 设置存储依赖并返回自身（与 NewVoiceHandler 链式；保留旧构造签名不破坏调用点）。
func (h *VoiceHandler) WithStorage(storage storageClient) *VoiceHandler {
	h.storage = storage
	return h
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

	// E2E-F-109：必须用 session.WithRequestAuth(c) 把 X-User-Id 注入 ctx，
	// 否则 ai-svc gRPC 拦截器（emotion-echo-ai-svc/internal/grpcserver/server.go:85）
	// 会以 "missing x-user-id metadata" 拒请求。BFF 主干约定（avatar_handler.go:113、
	// chat_handler.go 全部）都是这么写的，本 handler 是 Sprint 1 新增、当时无 ai-svc 故漏。
	authCtx := session.WithRequestAuth(c)

	// 调 ai-svc multimodal kind=audio
	resp, err := h.ai.MultiModalAnalyze(authCtx, downstream.MultiModalAnalyzeReq{
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

	// D-11：音频落 MinIO（voice/ 前缀）。**必须在 ai 调用成功之后做**，避免无意义写入；
	// storage 未配置 → 503（防止"录音看似成功但回放空白"的不可见失败）。
	if h.storage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code": 1, "message": "object storage not configured",
		})
		return
	}
	voiceKey := fmt.Sprintf("voice/%s-%s.webm", conversationID, uuid.NewString())
	// file 已被 ai-svc 读过，可能指针已耗尽——重新打开
	file2, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "reopen file: " + err.Error()})
		return
	}
	defer file2.Close()
	audioURL, err := h.storage.PutObject(authCtx, voiceKey, file2, fileHeader.Size, "audio/webm")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "storage put: " + err.Error()})
		return
	}

	// E2E-F-103：必须走 OK() 包装 —— 前端 useApi 统一取 data.data，
	// 裸顶层 gin.H 会让前端拿到 undefined（录音后整条链路静默无反馈）。
	OK(c, gin.H{
		"messageId":      uuid.NewString(), // 临时 ID；后续 chat-svc 持久化后替换
		"transcript":     resp.Transcript,
		"emotion":        resp.Emotion,
		"audioUrl":       audioURL, // D-11：语音气泡可回放的 URL
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