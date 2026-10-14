// Package handler — conversation_title.go
//
// CL-04 · T-18（2026-10-14 用户实测后追加）：**会话标题按第一轮对话自动概括**。
//
// 三层现状（账本 CL-F-12）：前端只在 [id].vue 显式传 `shouldGenerateTitle: false`、新建会话恒 undefined；
// BFF 请求结构里根本没有该字段（静默丢弃）；chat-svc 有 `UpdateTitle` 能力但无调用方（backlog R-13 实证复活）。
//
// 本文件的设计边界（**只动 A 轨文件面** —— 不碰 chat-svc / chat_handler / 前端）：
//   · 触发：前端显式 `shouldGenerateTitle: true`，或"落库后该会话消息数 ≤ 2"（= 第一轮对话）；
//   · 取标题：LLM（既有 ChatCompletion RPC，低温度 + 小 max_tokens）→ 失败/无 LLM 则**规则兜底**；
//   · 落地：调用**既有** `downstream.ChatClient.UpdateConversation`（不改其实现、不改 main.go 装配）；
//   · 全程 **best-effort**：任一依赖缺失/报错/超时都只是"不改标题"，绝不阻断对话、不冒泡。
package handler

import (
	"context"
	"log/slog"
	"strings"

	"emotion-echo-web-bff/internal/downstream"
)

// titleConvUpdater 只依赖"改会话标题"这一能力（窄接口 ⇒ 测试替身不必实现整个 ChatClient）。
type titleConvUpdater interface {
	UpdateConversation(ctx context.Context, conversationID int64, title string) (*downstream.UpdateConversationResp, error)
}

// firstExchangeMessageLimit 判定"是否第一轮对话"的取数上限：落库后 ≤ 2 条
// （1 条 user + 1 条 assistant）即第一轮；多取一条只为区分"恰好 2 条"与"更多"。
const firstExchangeMessageLimit = 3

// maybeGenerateTitle 第一轮对话后按对话内容生成会话标题。
//
// explicit = 前端显式要求（`shouldGenerateTitle`）。谁先满足都触发；都不满足则跳过（避免每轮重命名）。
func (h *AIStreamHandler) maybeGenerateTitle(ctx context.Context, conversationID int64, userContent string, explicit bool) {
	if h.titleUpdater == nil || conversationID <= 0 {
		return // 未装配改标题能力 ⇒ 增强不生效，不影响对话
	}
	if !explicit && !h.isFirstExchange(ctx, conversationID) {
		return
	}

	title := h.llmTitle(ctx, userContent)
	if title == "" {
		title = sanitizeTitle(userContent) // 规则兜底：取首条用户消息前若干字
	}
	if title == "" {
		return
	}

	if _, err := h.titleUpdater.UpdateConversation(ctx, conversationID, title); err != nil {
		slog.ErrorContext(ctx, "ai-stream update conversation title failed",
			"conversation_id", conversationID, "err", err)
	}
}

// isFirstExchange 会话（落库后）消息数 ≤ 2 ⇒ 第一轮对话。
// 取数失败 → 视为"不是第一轮"（保守：宁可不改名，也不误改历史会话标题）。
func (h *AIStreamHandler) isFirstExchange(ctx context.Context, conversationID int64) bool {
	if h.files == nil {
		return false
	}
	msgs, _, err := h.files.ListMessages(ctx, conversationID, firstExchangeMessageLimit, 0)
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream list messages for title check failed", "err", err)
		return false
	}
	return len(msgs) <= 2
}

// llmTitle 让 LLM 生成短标题；不可用/报错/结果为空 → 返回 ""（调用方走规则兜底）。
func (h *AIStreamHandler) llmTitle(ctx context.Context, userContent string) string {
	if h.llm == nil || strings.TrimSpace(userContent) == "" {
		return ""
	}
	var out strings.Builder
	err := h.llm.StreamChat(ctx, downstream.LLMStreamRequest{
		Model: h.cfg.LLM.Model,
		Messages: []downstream.Message{
			{Role: "system", Content: titleInstruction},
			{Role: "user", Content: userContent},
		},
		Temperature: titleTemperature,
		MaxTokens:   titleMaxTokens,
	}, func(delta, _ string) { out.WriteString(delta) })
	if err != nil {
		slog.ErrorContext(ctx, "ai-stream generate title via llm failed, falling back to rule", "err", err)
		return ""
	}
	return sanitizeTitle(out.String())
}

// sanitizeTitle 清洗标题：去空白/换行、去包裹引号书名号、去「标题：」前缀、去句末标点、按 rune 截断。
//
// 为什么必须清洗：LLM 常回「标题：xxx。」「"xxx"」这类包装；直接入库会让列表里出现怪东西。
func sanitizeTitle(s string) string {
	t := strings.TrimSpace(s)
	t = strings.ReplaceAll(t, "\n", " ")
	t = strings.TrimSpace(t)
	t = strings.Trim(t, "「」“”\"'《》")
	t = strings.TrimSpace(t)
	for _, prefix := range []string{"标题：", "标题:", "会话标题：", "会话标题:"} {
		t = strings.TrimPrefix(t, prefix)
	}
	t = strings.TrimSpace(t)
	t = strings.TrimRight(t, "。．.!！?？,，、;；:：")
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if r := []rune(t); len(r) > maxTitleRunes {
		t = strings.TrimSpace(string(r[:maxTitleRunes]))
	}
	return t
}
