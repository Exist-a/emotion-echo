// Package handler — ai_stream_handler.go
//
// Stage 30 / stage-30-web-bff.md T3.29-30 + 前端契约对齐: ai_stream handler
//
// 端点：POST /api/v1/ai/stream
// 请求（OpenAI chat.completions 兼容，前端 useAIStream.ts 发送）：
//   {"model": "...", "messages": [{"role":"user","content":"..."}], "stream": true}
// 响应：SSE 流（OpenAI 格式）
//   data: {"choices":[{"delta":{"content":"..."}}]}
//   ...
//   data: [DONE]
//
// 当前为 mock 实现（无真实 LLM 对话）：按关键词给出共情回复 + 情绪标签。
// 真实 LLM 对话流后续接 llm-service / ai-svc（保留 OpenAI 兼容格式，前端不变）。
//
// SSE headers：Content-Type: text/event-stream + X-Accel-Buffering: no（防缓冲）。
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/logging"

	"github.com/gin-gonic/gin"
)

// AIStreamHandler 是 /api/v1/ai/stream 的处理逻辑
type AIStreamHandler struct {
	cfg config.Config
}

// NewAIStreamHandler 构造 handler（返回 gin.HandlerFunc）
func NewAIStreamHandler(cfg config.Config) gin.HandlerFunc {
	h := &AIStreamHandler{cfg: cfg}
	return h.ServeHTTP
}

// aiStreamReq 兼容两种前端请求格式：
//   1. OpenAI 兼容（useAIStream.ts）：{"model","messages":[{"role","content"}],"stream"}
//   2. 发消息流程（useConversationSender）：{"message","emotion","conversationId"}
type aiStreamReq struct {
	Model          string `json:"model"`
	Messages       []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Stream         bool   `json:"stream"`
	Message        string `json:"message"`
	Emotion        string `json:"emotion"`
	ConversationID string `json:"conversationId"`
}

// ServeHTTP 处理 AI 对话流式回复
func (h *AIStreamHandler) ServeHTTP(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var req aiStreamReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "validation: invalid body", "data": nil})
		return
	}

	// 提取 user 消息内容（两种格式）
	userContent := req.Message
	if userContent == "" {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				userContent = req.Messages[i].Content
				break
			}
		}
	}
	if userContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "validation: messages is required", "data": nil})
		return
	}

	// mock 回复（按情绪关键词给出共情话术）
	reply := mockEmpathyReply(userContent)

	// SSE 逐块输出（OpenAI 格式）
	flusher, _ := c.Writer.(http.Flusher)
	writeDelta := func(content string) error {
		payload, err := json.Marshal(map[string]any{
			"choices": []map[string]any{{
				"delta": map[string]any{"content": content},
			}},
		})
		if err != nil {
			return err
		}
		if _, err := io.WriteString(c.Writer, "data: "+string(payload)+"\n\n"); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	// Phase D：APIKey 非空时调真实 LLM（DeepSeek / OpenAI 兼容）
	if h.cfg.LLM.APIKey != "" {
		llmMessages := []downstream.Message{
			{Role: "system", Content: "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"},
			{Role: "user", Content: userContent},
		}
		llmReq := downstream.LLMChatReq{
			Model:    h.cfg.LLM.Model,
			Messages: llmMessages,
		}
		llmClient := downstream.NewLLMClient(downstream.LLMOptions{
			BaseURL: h.cfg.LLM.BaseURL,
			APIKey:  h.cfg.LLM.APIKey,
			Model:   h.cfg.LLM.Model,
			Timeout: time.Duration(h.cfg.LLM.Timeout) * time.Second,
		})
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		err := llmClient.ChatStream(ctx, llmReq, func(content string) {
			if writeErr := writeDelta(content); writeErr != nil {
				logging.Errorf(writeErr, "[ai-stream] write LLM delta failed")
				cancel()
			}
		})
		if err != nil {
			logging.Errorf(err, "[ai-stream] LLM stream failed")
			_ = writeDelta("\n\n[抱歉，AI 服务暂时不可用]")
		}
		_, _ = io.WriteString(c.Writer, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	// Fallback：APIKey 为空 → mock 共情回复（dev 友好）
	_ = reply

	// 分块输出（按 rune 切，避免切断 UTF-8 中文；每 2 个字符一块模拟打字机）
	runes := []rune(reply)
	const chunkSize = 2
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if err := writeDelta(string(runes[i:end])); err != nil {
			logging.Errorf(err, "[ai-stream] write delta failed")
			return
		}
	}
	// 结束标记
	if _, err := io.WriteString(c.Writer, "data: [DONE]\n\n"); err != nil {
		logging.Errorf(err, "[ai-stream] write done failed")
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// mockEmpathyReply 按用户消息关键词生成共情回复（mock，真实 LLM 后续替换）
func mockEmpathyReply(content string) string {
	text := strings.ToLower(content)
	switch {
	case strings.Contains(text, "开心") || strings.Contains(text, "高兴") || strings.Contains(text, "棒") || strings.Contains(text, "好"):
		return "看到你这么开心，我也很为你高兴！能说说今天发生了什么让你心情这么好呀？"
	case strings.Contains(text, "难过") || strings.Contains(text, "伤心") || strings.Contains(text, "哭") || strings.Contains(text, "糟糕"):
		return "抱抱你，难过的时候确实很难受。我在这里陪着你，愿意的话可以慢慢说给我听。"
	case strings.Contains(text, "焦虑") || strings.Contains(text, "紧张") || strings.Contains(text, "担心"):
		return "听起来你现在有些焦虑。先深呼吸一下，我们一起把让你担心的事理一理好吗？"
	case strings.Contains(text, "累") || strings.Contains(text, "疲惫"):
		return "辛苦啦，累了就休息一下。你的感受很重要，我随时在这里听你说。"
	default:
		return "嗯，我在认真听你说。能多和我分享一些你的感受吗？"
	}
}
