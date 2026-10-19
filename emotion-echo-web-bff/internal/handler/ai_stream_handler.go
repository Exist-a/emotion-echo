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
	ListMessages(ctx context.Context, conversationID int64, limit int, cursor int64) ([]downstream.MessageView, bool, error)
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

// agentProfileSource 当前用户的角色档案来源（CL-06）。
//
// 取数失败 / 无档案 ⇒ 调用方**不注入角色段也不盖角色戳**（增强不是依赖，同 personalitySource 纪律）。
// 只要 List 一个方法 ⇒ 测试替身极小；downstream.AgentProfileClient 天然满足它。
type agentProfileSource interface {
	ListAgentProfiles(ctx context.Context) ([]downstream.AgentProfile, error)
}

// maxFileAttachments 每次 ai/stream 注入的最新文件消息上限（控制拉取/抽取开销）
const maxFileAttachments = 2

// system prompt 的组装已收敛到 `prompt_assembly.go`（CL-04 Phase 1，缺陷 11）：
// 段序、段文案与"无信号时逐字回落基础人设"的不变量都在那一处，两条上游路径共用。
//
// **文案与契约常量**（基础人设 / 适配表 / 情绪段句式 / 中文情绪映射）已集中到
// `prompt_templates.go`（CL-04 Phase 4）；本文件只留"怎么拼"的行为。
// 相关契约：D-14（face/voice 情绪上下文，E2E-16 plan §B.10；三句护栏：不点破来源 /
// 不贴标签 / 不过火）、E2E-F-122（payload 空时回落会话情绪历史，前端优先）、
// E2E-F-95（画像缺时不编造、静默回落基础人设）。

// buildEmotionHistoryContext 拼"最近情绪模式"段（E2E-F-122 回落路径）。
//
// 与实时段（buildEmotionContext 的"此刻神情/语气"）刻意区分：历史统计不能冒充
// 此刻信号（诚实性延伸自 D-14 不贴标签护栏）。句式：
// "情绪上下文：对方最近的状态多为X。请让回应贴合对方最近的状态。"
func buildEmotionHistoryContext(emotion string) string {
	if emotion == "" {
		return ""
	}
	return emotionSegmentPrefix + emotionHistoryPhrase + emotionChineseLabel(emotion) + emotionHistorySuffix
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
		parts = append(parts, emotionFacePhrase+emotionChineseLabel(faceEmotion))
	}
	if voiceEmotion != "" {
		parts = append(parts, emotionVoicePhrase+emotionChineseLabel(voiceEmotion))
	}
	return emotionSegmentPrefix + strings.Join(parts, emotionPhraseJoiner) + emotionNowSuffix
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
	// titleUpdater 是会话标题更新能力（T-18）；nil = 不生成标题（默认由 Chat 推断，见构造函数）
	titleUpdater titleConvUpdater
	// agents 是当前用户的角色档案来源（CL-06 用户自定义 Agent）；nil = 不注入角色段、不盖角色戳
	agents agentProfileSource
	// memory 是用户级记忆摘要来源（CL-08a）；nil = 不注入记忆段（= 现状行为）
	memory downstream.MemorySummarySource
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
	// TitleUpdater 是会话标题更新能力（T-18）。通常**不必显式传**：
	// 构造函数会从 Chat 推断（Chat 已满足该窄接口）；显式传只为测试替身。
	TitleUpdater titleConvUpdater
	// AgentProfiles 是角色档案来源（CL-06）；nil = 无角色段（= 现状行为，零回归）
	AgentProfiles agentProfileSource
	// Memory 是用户级记忆摘要来源（CL-08a）；nil = 无记忆段（= 现状行为，零回归）
	Memory downstream.MemorySummarySource
}

// NewAIStreamHandler 构造 handler（返回 gin.HandlerFunc）
func NewAIStreamHandler(cfg config.Config) gin.HandlerFunc {
	return NewAIStreamHandlerWithDeps(cfg, AIStreamDeps{})
}

// NewAIStreamHandlerWithDeps 构造带完整依赖的 handler。
// 零值 AIStreamDeps{} 等价于"仅 mock 回复"。
func NewAIStreamHandlerWithDeps(cfg config.Config, deps AIStreamDeps) gin.HandlerFunc {
	h := &AIStreamHandler{
		cfg:          cfg,
		llm:          deps.LLM,
		files:        deps.Files,
		chat:         deps.Chat,
		personality:  deps.Personality,
		emotion:      deps.Emotion,
		titleUpdater: deps.TitleUpdater,
		agents:       deps.AgentProfiles,
		memory:       deps.Memory,
	}
	// T-18：未显式传标题更新器时，从 Chat 推断（生产侧 Chat 已满足该窄接口 ⇒ main.go 无需改动装配）
	if h.titleUpdater == nil {
		if u, ok := deps.Chat.(titleConvUpdater); ok {
			h.titleUpdater = u
		}
	}
	return h.ServeHTTP
}

// resolveAgentProfile 解析本轮**生效**的用户自定义角色（CL-06 · 决议 CL-D-30）。
//
// 输入是请求携带的 `agentId`；输出一律是"本轮提示词用谁、写库就记谁"的那一份档案。
// 解析规则（顺序即优先级）：
//  1. 无角色来源 / 取数失败 / 用户一份角色都没有 ⇒ nil（**无角色段、不盖戳**；增强不是依赖）；
//  2. 请求指定的 id ∈ 自己的角色列表 ⇒ 用它（客户端传的 id 必须落在**自己的**列表里 ⇒
//     传别人的角色 id 取不到任何东西，等价于未指定，不构成越权）；
//  3. 请求指定的 id 找不到（已删 / 非本人）⇒ **回落默认角色**（T-4：不报错、不给空 prompt）；
//  4. 未指定（0）⇒ 默认角色（`isDefault`），无该标记时取列表首项兜底。
//
// 为什么不让 BFF 反过来读会话的 agent_id：那需要给 chat-svc 新增"单会话查询"RPC，
// 且仍要去 user-svc 取档案内容（两次调用）；"请求携带 + 本地列表校验"一次调用即可，
// 语义还与"这条回复实际由谁产生"天然一致（快照与人设同源，不会 A 答 B 标）。
func (h *AIStreamHandler) resolveAgentProfile(ctx context.Context, requestedID int64) *downstream.AgentProfile {
	if h.agents == nil {
		return nil
	}
	profiles, err := h.agents.ListAgentProfiles(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream list agent profiles failed, skipping agent persona", "err", err)
		return nil
	}
	if len(profiles) == 0 {
		return nil
	}
	if requestedID > 0 {
		for i := range profiles {
			if profiles[i].ID == requestedID {
				return &profiles[i]
			}
		}
	}
	return defaultAgentProfile(profiles)
}

// defaultAgentProfile 取默认角色（`isDefault`）；无该标记时取首项兜底。
//
// 兜底取首项而非返回 nil：列表非空却无 `isDefault` 属上游数据异常，
// 此时"用列表第一个"比"整轮没有人设"更接近用户预期（user-svc 侧正常恒有默认「小E」）。
func defaultAgentProfile(profiles []downstream.AgentProfile) *downstream.AgentProfile {
	for i := range profiles {
		if profiles[i].IsDefault {
			return &profiles[i]
		}
	}
	return &profiles[0]
}

// agentIDOf 取档案 id（nil ⇒ 0 = 不盖角色戳）
func agentIDOf(p *downstream.AgentProfile) int64 {
	if p == nil {
		return 0
	}
	return p.ID
}

// saveAIMessage 在 SSE 流完成后保存 AI 回复到 chat-svc（Stage 109b 修复）。
// 同步写（用 request ctx + session.WithRequestAuth 保留 x-user-id 注入 gRPC metadata），
// 失败仅记日志不阻断 SSE 已发出的响应。
//
// CL-06：额外记 `agentID` —— 写库时该会话的角色快照（0 = 无角色 ⇒ 落 NULL）。
func (h *AIStreamHandler) saveAIMessage(c *gin.Context, conversationID, content string, agentID int64) {
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
		AgentID: agentID, // CL-06：这条回复由哪个角色产生
	})
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "ai-stream save AI message failed",
			"conversation_id", convID, "agent_id", agentID, "err", err)
	}
}

// fileSourceURL 把消息里存的对象 URL 重写为 llm-service 容器内可达的内部端点。
// 两种落库形态都要覆盖（E2E-27 M1 / F-116）：
//   - 新形态：网关相对 /api/v1/uploads/file/<name>（M1 起）→ 内部端点直译
//   - 存量形态：PublicBaseURL 绝对地址（惰性兼容，不回填）
//
// 前缀均不匹配时原样返回（llm-service 侧 FILE_FETCH_ALLOWLIST 只放行
// emotion-echo-minio:9000 / localhost:9000 / 127.0.0.1:9000，见 file_context.py:24，
// 不匹配即拒绝并降级为"附件未能读取"）。
func fileSourceURL(publicURL string, cfg config.Config) string {
	scheme := "http"
	if cfg.MinIO.UseSSL {
		scheme = "https"
	}
	if cfg.MinIO.Endpoint == "" {
		return publicURL
	}
	// 新形态：网关相对路径 → /<bucket>/uploads/<name>（bucket 用配置值）
	const relPrefix = "/api/v1/uploads/file/"
	if strings.HasPrefix(publicURL, relPrefix) {
		return scheme + "://" + cfg.MinIO.Endpoint + "/" + cfg.MinIO.Bucket + "/uploads/" +
			strings.TrimPrefix(publicURL, relPrefix)
	}
	// 存量形态：PublicBaseURL 前缀重写
	base := cfg.MinIO.PublicBaseURL
	if base == "" || !strings.HasPrefix(publicURL, base) {
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
	msgs, _, err := h.files.ListMessages(ctx, convID, 50, 0)
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
	// AgentID CL-06：本会话当前角色（前端切换器带上来；0 = 未指定 ⇒ 用用户默认角色）。
	// ⚠️ 客户端传值**不可信** —— 只当"想要哪一个"，实际生效值一律经 resolveAgentProfile
	// 在**自己的**角色列表里解析（不存在 ⇒ 回落默认）。
	AgentID int64 `json:"agentId"`
	// ShouldGenerateTitle T-18：前端显式要求"生成/重命名会话标题"。
	// 此前该字段**不在结构里** ⇒ 静默丢弃（Phase 0 缺陷 8 的一项）；现接住并参与触发条件。
	ShouldGenerateTitle bool `json:"shouldGenerateTitle"`
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

	// CL-06：解析本轮生效角色（llm-gRPC / HTTP 直连 / mock 三条路径共用同一份）。
	// 取数失败/无档案 ⇒ nil ⇒ 不注入角色段也不盖戳（与 personality/emotion 同纪律）。
	agent := h.resolveAgentProfile(session.WithRequestAuth(c), req.AgentID)

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
		llmMessages := h.assembleMessages(assembleInput{
			ctx:             session.WithRequestAuth(c),
			conversationID:  parseConversationID(req.ConversationID),
			faceEmotion:     req.FaceEmotion,
			faceConfidence:  req.FaceConfidence,
			voiceEmotion:    req.VoiceEmotion,
			voiceConfidence: req.VoiceConfidence,
			textEmotion:     strings.TrimSpace(req.Emotion), // CL-04 Phase 6：文本情绪（neutral=无信号）
			userContent:     userContent,
			agent:            agent, // CL-06：用户自定义角色（两处必须都传）
			memory:           h.loadMemorySummary(session.WithRequestAuth(c)), // CL-08a：记忆摘要（两处必须都传）
		})
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		// Stage 89 PR-3：会话内文件持续引用——最新 ≤2 条 file 消息随请求下发，
		// llm-service 负责拉取/抽取/注入 system 上下文
		// Stage 89 PR-6 e2e 揪出：必须用 auth ctx（注入 x-user-id），否则 chat gRPC ListMessages 鉴权拒
		files := h.collectFileAttachments(session.WithRequestAuth(c), req.ConversationID)
		// Sprint 109b：累计完整 AI 回复文本，stream 完后异步写库
		var llmAccumulated strings.Builder
		err := h.llm.StreamChat(ctx, downstream.LLMStreamRequest{
			Model:       h.cfg.LLM.Model,
			Messages:    llmMessages,
			Files:       files,
			Temperature: llmGeneration.Temperature,
			MaxTokens:   llmGeneration.MaxTokens,
			WithIntent:  llmWithIntent, // CL-04 Phase 5：接回意图风格注入（缺陷 5）
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
			h.saveAIMessage(c, req.ConversationID, llmAccumulated.String(), agentIDOf(agent))
			// T-18：第一轮对话后按对话内容生成会话标题（best-effort，不影响已发出的 SSE）
			h.maybeGenerateTitle(session.WithRequestAuth(c),
				parseConversationID(req.ConversationID), userContent, req.ShouldGenerateTitle)
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
		llmMessages := h.assembleMessages(assembleInput{
			ctx:             session.WithRequestAuth(c),
			conversationID:  parseConversationID(req.ConversationID),
			faceEmotion:     req.FaceEmotion,
			faceConfidence:  req.FaceConfidence,
			voiceEmotion:    req.VoiceEmotion,
			voiceConfidence: req.VoiceConfidence,
			textEmotion:     strings.TrimSpace(req.Emotion), // CL-04 Phase 6：文本情绪（neutral=无信号）
			userContent:     userContent,
			agent:            agent, // CL-06：用户自定义角色（两处必须都传）
			memory:           h.loadMemorySummary(session.WithRequestAuth(c)), // CL-08a：记忆摘要（两处必须都传）
		})
		llmReq := downstream.LLMChatReq{
			Model:       h.cfg.LLM.Model,
			Messages:    llmMessages,
			Temperature: llmGeneration.Temperature,
			MaxTokens:   llmGeneration.MaxTokens,
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
		h.saveAIMessage(c, req.ConversationID, llmAccumulated.String(), agentIDOf(agent))
		h.maybeGenerateTitle(session.WithRequestAuth(c),
			parseConversationID(req.ConversationID), userContent, req.ShouldGenerateTitle)
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
	h.saveAIMessage(c, req.ConversationID, reply, agentIDOf(agent))
	h.maybeGenerateTitle(session.WithRequestAuth(c),
		parseConversationID(req.ConversationID), userContent, req.ShouldGenerateTitle)
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
