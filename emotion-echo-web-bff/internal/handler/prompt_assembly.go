// Package handler — prompt_assembly.go
//
// CL-04 · Phase 1（A 轨，2026-10-14）：**统一 Prompt 组装层**（先"换实现不改行为"）。
//
// 为什么（缺陷 11）：原实现把 system prompt 的拼装写在 handler 里，且在 ServeHTTP 内
// 有**两处等价内联调用点**（llm-gRPC 路径 与 HTTP 直连路径各拼一次）——
// 任何一段措辞/顺序的改动都要改两处，漏一处即路径间漂移（且没有任何门禁能发现）。
//
// 设计（plan §3.1 分层组装模型的最小落地）：
//   - 每段由独立 builder 产出（纯函数，可单独断言"有/无/顺序"）；
//   - 段序由 `systemSegments` 唯一决定：**base 恒在首**，其后按 canonical 次序；
//     **护栏类段（将来）必须排在用户可控段之后**（防越狱）——T-2 钉死；
//   - 组装层是**唯一**产出 messages 的地方，两条上游路径共用（T-8 钉死）。
//
// Phase 1 边界：只做"换实现不改行为"——段集合、措辞、分隔符与旧实现**逐字一致**
// （T-1 用改造前旧实现产出的 golden 字面量钉死）。历史注入（Phase 2）、
// 预算/参数（Phase 3）、配置化（Phase 4）在其后叠加，每步仍受 T-1 保护。
package handler

import (
	"context"
	"log/slog"
	"strings"

	"emotion-echo-web-bff/internal/downstream"
)

// 段类型。**顺序即 canonical 顺序**（base 恒首；护栏类段必须排在用户可控段之后）。
const (
	segmentKindBase        = "base"
	segmentKindPersonality = "personality"
	segmentKindEmotion     = "emotion"
)

// segmentSeparator 段间分隔符 —— 与旧实现的 `base + "\n\n" + seg` 保持一致（T-1 逐字回归的前提）。
const segmentSeparator = "\n\n"

// promptSegment 组装层的一段。Kind 供段序断言；Text 为空时该段被丢弃（不产生空行）。
type promptSegment struct {
	Kind string
	Text string
}

// assembleInput 组装输入。
//
// Phase 1 只有 system 段所需的信号；Phase 2 会加 history（同会话历史），
// Phase 3 会加预算/参数。userContent 单独放在此，使"完整 messages"由同一处产出。
type assembleInput struct {
	ctx context.Context
	// conversationID > 0 才可能回落会话情绪历史（0 = 无会话上下文）
	conversationID  int64
	faceEmotion     string
	faceConfidence  float64
	voiceEmotion    string
	voiceConfidence float64
	// textEmotion 当前轮用户消息的情绪标签（前端 `emotion` 字段，CL-04 Phase 6 接上）。
	// `neutral` / 空 = 无信号（见 defaultTextEmotion —— 线上该字段目前恒为默认值）。
	textEmotion string
	// userContent 当前轮用户输入（Phase 2 后历史会排在它之前）
	userContent string
}

// systemSegments 产出 system 段的**有序**列表（空段不出现）。
//
// 顺序不变量（T-2）：base → personality → emotion；新增段一律追加在 canonical 次序之内，
// 用户可控内容（CL-06 的 Agent 人设）之后必须有护栏段兜底。
func (h *AIStreamHandler) systemSegments(in assembleInput) []promptSegment {
	segs := []promptSegment{{Kind: segmentKindBase, Text: baseSystemPrompt}}
	if guide := h.personalitySegment(in.ctx); guide != "" {
		segs = append(segs, promptSegment{Kind: segmentKindPersonality, Text: guide})
	}
	if emotionSeg := h.emotionSegment(in); emotionSeg != "" {
		segs = append(segs, promptSegment{Kind: segmentKindEmotion, Text: emotionSeg})
	}
	return segs
}

// personalitySegment 产出人格「相处方式适配」段。
//
// 无画像来源 / 来源报错 / 画像平坦无信息（见 buildPersonalityGuide）→ 返回 ""，
// 调用方回落基础人设 —— **画像注入是增强不是依赖，不编造**（E2E-F-95 教训）。
func (h *AIStreamHandler) personalitySegment(ctx context.Context) string {
	if h.personality == nil {
		return ""
	}
	dims, err := h.personality.LatestPersonalityProfile(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream load personality profile failed, using base prompt", "err", err)
		return ""
	}
	return buildPersonalityGuide(dims)
}

// emotionSegment 产出情绪上下文段。
//
// 优先级（同为"当前轮信号"，按可靠度排）：
//
//	face/voice（多模态实时）→ textEmotion（当前轮文本标签）→ 会话情绪历史（统计回落）
//
// 前两者**不叠加**：face/voice 任一非空时不再用文本信号（避免把"神情"与"话语"拼成可能矛盾的描述）。
// 历史查询报错/为空 → 返回 ""（增强不是依赖）。
func (h *AIStreamHandler) emotionSegment(in assembleInput) string {
	if seg := buildEmotionContext(in.faceEmotion, in.faceConfidence, in.voiceEmotion, in.voiceConfidence); seg != "" {
		return seg
	}
	if seg := buildTextEmotionContext(in.textEmotion); seg != "" {
		return seg
	}
	if h.emotion == nil || in.conversationID <= 0 {
		return ""
	}
	pattern, err := h.emotion.RecentEmotionPattern(in.ctx, in.conversationID)
	if err != nil || pattern == "" {
		return ""
	}
	return buildEmotionHistoryContext(pattern)
}

// buildTextEmotionContext 拼"文本情绪"段（当前轮用户消息的情绪标签）。
//
// `neutral` 与空 = 无信号（`defaultTextEmotion` 的由来见 prompt_templates.go 注释）——
// 该值目前是前端默认值而非测量值，照单注入等于对每条消息断言"对方此刻平静"。
func buildTextEmotionContext(emotion string) string {
	e := strings.TrimSpace(emotion)
	if e == "" || e == defaultTextEmotion {
		return ""
	}
	return emotionSegmentPrefix + emotionTextPhrase + emotionChineseLabel(e) + emotionNowSuffix
}

// assembleSystemPrompt 把有序段拼成完整 system 串（空段已在上游丢弃）。
func (h *AIStreamHandler) assembleSystemPrompt(in assembleInput) string {
	segs := h.systemSegments(in)
	texts := make([]string, 0, len(segs))
	for _, s := range segs {
		if s.Text != "" {
			texts = append(texts, s.Text)
		}
	}
	return strings.Join(texts, segmentSeparator)
}

// assembleMessages 产出完整 messages —— **两条上游路径的唯一组装点**。
//
// 结构（CL-04 Phase 2 起）：
//
//	[system(段组装)] + [同会话历史：时间正序，窗口+预算裁剪] + [当前 user]
//
// 历史为空（无会话 / 无来源 / 取数失败 / 去重后为空）时退化为 Phase 1 的两条形态。
func (h *AIStreamHandler) assembleMessages(in assembleInput) []downstream.Message {
	history := h.historyMessages(in)
	msgs := make([]downstream.Message, 0, len(history)+2)
	msgs = append(msgs, downstream.Message{Role: "system", Content: h.assembleSystemPrompt(in)})
	msgs = append(msgs, history...)
	msgs = append(msgs, downstream.Message{Role: "user", Content: clampUserContent(in.ctx, in.userContent)})
	return msgs
}
