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

// emotionSource 会话最近情绪模式来源（E2E-F-122 D-14 高级模式）。
// 返回空串 / err 时调用方不注入情绪段 —— 情绪注入是增强不是依赖（与 personalitySource 同款纪律）。
type emotionSource interface {
	RecentEmotionPattern(ctx context.Context, conversationID int64) (string, error)
}

// maxFileAttachments 每次 ai/stream 注入的最新文件消息上限（控制拉取/抽取开销）
const maxFileAttachments = 2

// baseSystemPrompt 基础人设（无画像时的完整 prompt）
const baseSystemPrompt = "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"

// personalityDimensionLabels 五维度中文标签（顺序即画像摘要顺序）
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

// buildSystemPrompt 组装 system prompt：基础人设 + （可选）人格「联系方式适配」指令。
//
// 无画像 / 画像来源报错 / 画像平坦无信息（见 buildPersonalityGuide）时，
// 返回与无画像时**逐字相同**的基础人设 —— 不编造、不退化。
func (h *AIStreamHandler) buildSystemPrompt(ctx context.Context) string {
	if h.personality == nil {
		return baseSystemPrompt
	}
	dims, err := h.personality.LatestPersonalityProfile(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream load personality profile failed, using base prompt", "err", err)
		return baseSystemPrompt
	}
	guide := buildPersonalityGuide(dims)
	if guide == "" {
		return baseSystemPrompt
	}
	return baseSystemPrompt + "\n\n" + guide
}

// buildSystemPromptWithEmotion 组装 system prompt：基础人设 + （可选）人格段 + （可选）情绪上下文段。
//
// D-14（E2E-16 plan §B.10）：前端携带 face / voice 情绪上下文（faceEmotion/voiceEmotion），
// 拼到 system prompt 让 AI 回复贴合用户当前状态。
// 三句护栏（与 E2E-F-95 人格画像同款约束）：
//   - 不点破来源（不出现"摄像头/识别/分析/检测/设备/传感器/面部识别"）
//   - 不贴标签（不当面称呼"你很happy"等）
//   - 不过火（情绪描述保持中性、简短）
// 无情绪上下文时 emotionSeg 为空 → 返回与原 buildSystemPrompt 逐字相同（不污染）。
//
// E2E-F-122（emotionSource 高级模式）：face/voice 全空时（摄像头关闭 / 权限被拒 /
// 3 秒窗口过期）回落 emotionSource 查会话情绪历史，注入"最近情绪模式"段。
// 前端 payload 优先 —— 实时信号 > 历史统计，非空时不查（省一次 gRPC）。
func (h *AIStreamHandler) buildSystemPromptWithEmotion(
	ctx context.Context,
	conversationID int64,
	faceEmotion string, faceConfidence float64,
	voiceEmotion string, voiceConfidence float64,
) string {
	base := h.buildSystemPrompt(ctx)
	emotionSeg := buildEmotionContext(faceEmotion, faceConfidence, voiceEmotion, voiceConfidence)
	if emotionSeg == "" && h.emotion != nil && conversationID > 0 {
		if pattern, err := h.emotion.RecentEmotionPattern(ctx, conversationID); err == nil && pattern != "" {
			emotionSeg = buildEmotionHistoryContext(pattern)
		}
	}
	if emotionSeg == "" {
		return base
	}
	return base + "\n\n" + emotionSeg
}

// buildEmotionHistoryContext 拼"最近情绪模式"段（E2E-F-122 回落路径）。
//
// 与实时段（buildEmotionContext 的"此刻神情/语气"）刻意区分：历史统计不能冒充
// 此刻信号（诚实性延伸自 D-14 不贴标签护栏）。句式：
// "情绪上下文：对方最近的状态多为X。请让回应贴合对方最近的状态。"
func buildEmotionHistoryContext(emotion string) string {
	if emotion == "" {
		return ""
	}
	return "情绪上下文：对方最近的状态多为" + emotionChineseLabel(emotion) + "。请让回应贴合对方最近的状态。"
}

// parseConversationID 把请求体的 conversationId 字符串解析为 int64。
// 空串 / 非数字 / 非正数 → 0（调用方以 0 表示"无会话上下文，不查历史"）。
func parseConversationID(raw string) int64 {
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// buildEmotionContext 拼"情绪上下文"段（face/voice 任一非空时返回非空字符串）。
//
// 句式："对方此刻神情看起来X，对方语气听起来Y。请让回应贴合对方此刻的状态。"
// 中文情绪映射：happy→愉快、sad→低落、angry→烦躁、anxious→不安、neutral→平静、其他→原样保留。
func buildEmotionContext(faceEmotion string, _ float64, voiceEmotion string, _ float64) string {
	if faceEmotion == "" && voiceEmotion == "" {
		return ""
	}
	var parts []string
	if faceEmotion != "" {
		parts = append(parts, "对方此刻神情看起来"+emotionChineseLabel(faceEmotion))
	}
	if voiceEmotion != "" {
		parts = append(parts, "对方语气听起来"+emotionChineseLabel(voiceEmotion))
	}
	return "情绪上下文：" + strings.Join(parts, "，") + "。请让回应贴合对方此刻的状态。"
}

// emotionChineseLabel 把英文情绪标签映射为中文（避免在 prompt 里出现英文枚举值）。
func emotionChineseLabel(emotion string) string {
	switch emotion {
	case "happy":
		return "愉快"
	case "sad":
		return "低落"
	case "angry":
		return "烦躁"
	case "anxious":
		return "不安"
	case "neutral":
		return "平静"
	case "surprise":
		return "惊讶"
	case "fear":
		return "紧张"
	case "disgust":
		return "不适"
	default:
		// 未知情绪 → 原样返回（不会泄漏系统术语）
		return emotion
	}
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
	// emotion 是会话情绪历史来源（E2E-F-122）；nil = 不回落历史情绪
	emotion emotionSource
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
	// Emotion 是会话情绪历史来源（E2E-F-122）；nil = face/voice 空时不回落历史情绪
	Emotion emotionSource
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
		emotion:     deps.Emotion,
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
//
// D-14（E2E-16 plan §B.10）：新增 FaceEmotion / FaceConfidence / VoiceEmotion / VoiceConfidence
// —— 前端从 useFaceEmotion / useVoiceRecorder 取出最近一次结果随请求带上，由 BFF 拼入
// system prompt，让 AI 回复贴合用户当前情绪状态。
type aiStreamReq struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Stream         bool    `json:"stream"`
	Message        string  `json:"message"`
	Emotion        string  `json:"emotion"`
	ConversationID string  `json:"conversationId"`
	// D-14：face/voice 情绪上下文（前端从 useFaceEmotion / useVoiceRecorder 携带）
	FaceEmotion    string  `json:"faceEmotion"`
	FaceConfidence float64 `json:"faceConfidence"`
	VoiceEmotion   string  `json:"voiceEmotion"`
	VoiceConfidence float64 `json:"voiceConfidence"`
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
			{Role: "system", Content: h.buildSystemPromptWithEmotion(
				session.WithRequestAuth(c),
				parseConversationID(req.ConversationID),
				req.FaceEmotion, req.FaceConfidence,
				req.VoiceEmotion, req.VoiceConfidence,
			)},
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
			{Role: "system", Content: h.buildSystemPromptWithEmotion(
				session.WithRequestAuth(c),
				parseConversationID(req.ConversationID),
				req.FaceEmotion, req.FaceConfidence,
				req.VoiceEmotion, req.VoiceConfidence,
			)},
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
