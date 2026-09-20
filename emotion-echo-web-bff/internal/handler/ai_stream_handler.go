// Package handler — ai_stream_handler.go
//
// Stage 30 / stage-30-web-bff.md T3.29-30 + 前端契约对齐: ai_stream handler
//
// 端点：POST /api/v1/ai/stream
// 请求（OpenAI chat.completions 兼容，前端 useAIStream.ts 发送）：
//
//	{"model": "...", "messages": [{"role":"user","content":"..."}], "stream": true}
//
// 响应：SSE 流（OpenAI 格式）
//
//	data: {"choices":[{"delta":{"content":"..."}}]}
//	...
//	data: [DONE]
//
// 当前为 mock 实现（无真实 LLM 对话）：按关键词给出共情回复 + 情绪标签。
// 真实 LLM 对话流后续接 llm-service / ai-svc（保留 OpenAI 兼容格式，前端不变）。
//
// SSE headers：Content-Type: text/event-stream + X-Accel-Buffering: no（防缓冲）。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// fileMessageLister 会话消息列表来源（downstream.ChatClient 满足此接口）；
// 独立小接口便于测试 fake（Stage 89 PR-3）
type fileMessageLister interface {
	ListMessages(ctx context.Context, conversationID int64, limit int) ([]downstream.MessageView, error)
}

// personalitySource 用户最新人格画像来源（E2E-14）。
// 返回 nil（无结果）或 err 时，调用方回落基础人设 prompt —— 画像注入是增强不是依赖。
type personalitySource interface {
	LatestPersonalityProfile(ctx context.Context) (map[string]float64, error)
}

// maxFileAttachments 每次 ai/stream 注入的最新文件消息上限（控制拉取/抽取开销）
const maxFileAttachments = 2

// baseSystemPrompt 基础人设（无画像时的完整 prompt）
const baseSystemPrompt = "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"

// personalityDimensionLabels 五维度中文标签（顺序即画像文本顺序）
var personalityDimensionLabels = []struct {
	Key   string
	Label string
}{
	{"openness", "开放性"},
	{"conscientiousness", "尽责性"},
	{"extraversion", "外向性"},
	{"agreeableness", "宜人性"},
	{"neuroticism", "神经质"},
}

// formatPersonalityContext 把五维度分数格式化为可读画像文本。
// 维度分 6-30（每维度 6 题 × 1-5 分），18 为中性：≥23 高 / 14~22 中 / ≤13 低。
// 缺失维度跳过（不补 0 —— 补 0 会被读成"极低"，是编造画像）。空输入返回 ""。
func formatPersonalityContext(dims map[string]float64) string {
	if len(dims) == 0 {
		return ""
	}
	parts := make([]string, 0, len(personalityDimensionLabels))
	for _, d := range personalityDimensionLabels {
		score, ok := dims[d.Key]
		if !ok {
			continue
		}
		level := "中"
		switch {
		case score >= 23:
			level = "高"
		case score <= 13:
			level = "低"
		}
		parts = append(parts, fmt.Sprintf("%s%s（%.0f/30）", d.Label, level, score))
	}
	return strings.Join(parts, "，")
}

// buildSystemPrompt 组装 system prompt：基础人设 + （可选）人格画像。
// 画像来源为 nil / 报错 / 无结果时返回基础人设。
func (h *AIStreamHandler) buildSystemPrompt(ctx context.Context) string {
	if h.personality == nil {
		return baseSystemPrompt
	}
	dims, err := h.personality.LatestPersonalityProfile(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream load personality profile failed, using base prompt", "err", err)
		return baseSystemPrompt
	}
	profile := formatPersonalityContext(dims)
	if profile == "" {
		return baseSystemPrompt
	}
	return baseSystemPrompt +
		"\n\n用户人格画像（五因素量表，每维度 6-30 分，18 为中性）：" + profile +
		"。请在回应风格上贴合该画像，但不要直接点破你在套用测评结果。"
}

// AIStreamHandler 是 /api/v1/ai/stream 的处理逻辑
type AIStreamHandler struct {
	cfg config.Config
	// llm 是 llm-service ChatCompletion gRPC 上游（Stage 81 PR-2）；nil = 未装配（走 mock/HTTP 直连）
	llm downstream.LLMChatStreamer
	// files 是会话消息列表来源（Stage 89 PR-3）；nil = 不注入文件上下文
	files fileMessageLister
	// chat 是 chat-svc 客户端（Stage 109b 修复：SSE 流完后写 AI 回复到 chat-svc，
	// 防止前端刷新后 AI 回复丢失 — 之前 BFF 只走 SSE 流不存库，message 表只有 user msg）
	chat downstream.ChatClient
	// personality 是人格画像来源（E2E-14）；nil = 不注入人格画像
	personality personalitySource
}

// AIStreamDeps 是 AIStreamHandler 的依赖集合。
//
// 改用 options struct 而非逐个构造函数变体：依赖已增至 5 个，历史做法是每加一个
// 就新增一个 NewAIStreamHandlerWithXxx 并保留旧签名（测试在用），导致 5 个变体线性增长
// （E2E-F-92）。新增依赖现在只改本 struct + 调用点传字段，不再新增构造函数。
// 任一字段可为 nil：nil = 该能力未装配（handler 内部逐项判空降级）。
type AIStreamDeps struct {
	// LLM 是 llm-service ChatCompletion gRPC 上游；nil = 走 HTTP 直连 / mock
	LLM downstream.LLMChatStreamer
	// Files 是会话消息列表来源（文件上下文注入）；nil = 不注入文件
	Files fileMessageLister
	// Chat 是 chat-svc 客户端（SSE 流完后写 AI 回复入库）；nil = 不入库
	Chat downstream.ChatClient
	// Personality 是人格画像来源（E2E-14）；nil = 不注入人格画像
	Personality personalitySource
}

// NewAIStreamHandler 构造 handler（返回 gin.HandlerFunc）
func NewAIStreamHandler(cfg config.Config) gin.HandlerFunc {
	return NewAIStreamHandlerWithDeps(cfg, AIStreamDeps{})
}

// NewAIStreamHandlerWithDeps 构造带完整依赖的 handler。
// 零值 AIStreamDeps{} 等价于"仅 mock 回复"。
func NewAIStreamHandlerWithDeps(cfg config.Config, deps AIStreamDeps) gin.HandlerFunc {
	h := &AIStreamHandler{
		cfg:         cfg,
		llm:         deps.LLM,
		files:       deps.Files,
		chat:        deps.Chat,
		personality: deps.Personality,
	}
	return h.ServeHTTP
}

// saveAIMessage 在 SSE 流完成后保存 AI 回复到 chat-svc（Stage 109b 修复）。
// 同步写（用 request ctx + session.WithRequestAuth 保留 x-user-id 注入 gRPC metadata），
// 失败仅记日志不阻断 SSE 已发出的响应。
func (h *AIStreamHandler) saveAIMessage(c *gin.Context, conversationID, content string) {
	if h.chat == nil || conversationID == "" || strings.TrimSpace(content) == "" {
		return
	}
	convID, err := strconv.ParseInt(conversationID, 10, 64)
	if err != nil || convID <= 0 {
		return
	}
	// 必须用 auth ctx（注入 x-user-id），否则 chat-svc gRPC 鉴权拒
	// 见 emotion-echo-shared/pkg/middleware/grpc_userid.go
	_, err = h.chat.SendMessage(session.WithRequestAuth(c), convID, downstream.SendMessageReq{
		Role:    "assistant",
		Content: content,
	})
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "ai-stream save AI message failed",
			"conversation_id", convID, "err", err)
	}
}

// fileSourceURL 把消息里存的 MinIO 公开 URL（PublicBaseURL，面向浏览器）
// 重写为 llm-service 容器内可达的内部端点。前缀不匹配时原样返回
// （llm-service 侧 FILE_FETCH_ALLOWLIST 会拒绝并降级为"附件未能读取"）。
func fileSourceURL(publicURL string, cfg config.Config) string {
	base := cfg.MinIO.PublicBaseURL
	if base == "" || !strings.HasPrefix(publicURL, base) {
		return publicURL
	}
	scheme := "http"
	if cfg.MinIO.UseSSL {
		scheme = "https"
	}
	if cfg.MinIO.Endpoint == "" {
		return publicURL
	}
	return scheme + "://" + cfg.MinIO.Endpoint + strings.TrimPrefix(publicURL, base)
}

// collectFileAttachments 取会话最近消息中最新的 ≤2 条 file 消息转为附件引用。
// 拉取失败只记日志不阻断对话（文件上下文是增强，不是依赖）。
func (h *AIStreamHandler) collectFileAttachments(ctx context.Context, conversationID string) []downstream.FileAttachment {
	if h.files == nil || conversationID == "" {
		return nil
	}
	convID, err := strconv.ParseInt(conversationID, 10, 64)
	if err != nil || convID <= 0 {
		return nil
	}
	msgs, err := h.files.ListMessages(ctx, convID, 50)
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream list messages for file context failed", "err", err)
		return nil
	}
	var out []downstream.FileAttachment
	// msgs 按时间倒序（最新在前）；倒着收集保证 Files 顺序为旧→新
	for i := 0; i < len(msgs) && len(out) < maxFileAttachments; i++ {
		m := msgs[len(msgs)-1-i]
		if m.ContentType != "file" || m.Content == "" {
			continue
		}
		out = append(out, downstream.FileAttachment{
			URL:  fileSourceURL(m.Content, h.cfg),
			Name: m.FileName,
		})
	}
	// 上面循环按倒序从新到旧 append 后需反转回旧→新
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// aiStreamReq 兼容两种前端请求格式：
//  1. OpenAI 兼容（useAIStream.ts）：{"model","messages":[{"role","content"}],"stream"}
//  2. 发消息流程（useConversationSender）：{"message","emotion","conversationId"}
type aiStreamReq struct {
	Model    string `json:"model"`
	Messages []struct {
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

	flusher0, _ := c.Writer.(http.Flusher)
	writeDelta0 := func(content string) error {
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
		if flusher0 != nil {
			flusher0.Flush()
		}
		return nil
	}

	// Stage 81 PR-2：llm-service ChatCompletion gRPC 优先（LLM key 管理与降级收敛在
	// llm-service——PR-1；fallback_reason 随 delta 透传给前端日志）。传输失败回落
	// 既有 Phase D HTTP 直连 / mock。
	if h.llm != nil {
		llmMessages := []downstream.Message{
			{Role: "system", Content: h.buildSystemPrompt(session.WithRequestAuth(c))},
			{Role: "user", Content: userContent},
		}
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		// Stage 89 PR-3：会话内文件持续引用——最新 ≤2 条 file 消息随请求下发，
		// llm-service 负责拉取/抽取/注入 system 上下文
		// Stage 89 PR-6 e2e 揪出：必须用 auth ctx（注入 x-user-id），否则 chat gRPC ListMessages 鉴权拒
		files := h.collectFileAttachments(session.WithRequestAuth(c), req.ConversationID)
		// Sprint 109b：累计完整 AI 回复文本，stream 完后异步写库
		var llmAccumulated strings.Builder
		err := h.llm.StreamChat(ctx, downstream.LLMStreamRequest{
			Model:    h.cfg.LLM.Model,
			Messages: llmMessages,
			Files:    files,
		}, func(delta, model string) {
			llmAccumulated.WriteString(delta)
			if writeErr := writeDelta0(delta); writeErr != nil {
				slog.ErrorContext(c.Request.Context(), "ai-stream write llm-grpc delta failed", "err", writeErr)
				cancel()
			}
		})
		if err == nil {
			_, _ = io.WriteString(c.Writer, "data: [DONE]\n\n")
			if flusher0 != nil {
				flusher0.Flush()
			}
			h.saveAIMessage(c, req.ConversationID, llmAccumulated.String())
			return
		}
		slog.ErrorContext(c.Request.Context(), "ai-stream llm-grpc upstream failed, falling back", "err", err)
		// 重置已写内容不可行——SSE 已发出；此处仅记日志并走下方降级（delta 前置提示）
		_ = writeDelta0("[AI 上游切换] ")
	}

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
			{Role: "system", Content: h.buildSystemPrompt(session.WithRequestAuth(c))},
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
		var llmAccumulated strings.Builder
		err := llmClient.ChatStream(ctx, llmReq, func(content string) {
			llmAccumulated.WriteString(content)
			if writeErr := writeDelta(content); writeErr != nil {
				slog.ErrorContext(c.Request.Context(), "ai-stream write LLM delta failed", "err", writeErr)
				cancel()
			}
		})
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "ai-stream LLM stream failed", "err", err)
			_ = writeDelta("\n\n[抱歉，AI 服务暂时不可用]")
		}
		_, _ = io.WriteString(c.Writer, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		h.saveAIMessage(c, req.ConversationID, llmAccumulated.String())
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
			slog.ErrorContext(c.Request.Context(), "ai-stream write delta failed", "err", err)
			return
		}
	}
	// 结束标记
	if _, err := io.WriteString(c.Writer, "data: [DONE]\n\n"); err != nil {
		slog.ErrorContext(c.Request.Context(), "ai-stream write done failed", "err", err)
	}
	if flusher != nil {
		flusher.Flush()
	}
	// Sprint 109b: 异步保存 AI 回复到 chat-svc (防止刷新后丢失)
	h.saveAIMessage(c, req.ConversationID, reply)
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
