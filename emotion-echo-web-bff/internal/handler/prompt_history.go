// Package handler — prompt_history.go
//
// CL-04 · Phase 2（A 轨，2026-10-14）：**同会话历史注入**。
//
// 解决的核心缺陷：对话 prompt 只有 `[system, 最后一条 user]` ⇒ 多轮对话对 LLM 不可见
// （每轮都当陌生人重来），对"情绪疏导"这类天然多轮渐进的产品是致命伤（plan §1）。
//
// 用户 2026-10-14 裁定的参数（roadmap §4 CL-04 已定参数，[M] T-16 邻项）：
//   · 滑动窗口：**默认最近 10 条消息**、**上限 20 条**，可配 —— 配置值夹在 1..20；
//     **token 预算只做"缩"不做"扩"**；
//   · token 预算硬顶 **2000**（中文按 1 字 ≈ 1 token 估算，不追求精确，只需防爆）。
//
// 取数：复用 chat-svc `ListMessages(ctx, convID, limit, cursor)`（`fileMessageLister`）。
// 该接口返回**最新 limit 条、时间升序**（`conversation_repository.go:350-372` 已核）⇒
// 直接按序即是"旧→新"，无需再排。
package handler

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	"emotion-echo-web-bff/internal/downstream"
)

// 历史窗口与预算（用户 2026-10-14 裁定）。
const (
	// defaultHistoryMessages 默认注入的历史条数（不含当前轮）
	defaultHistoryMessages = 10
	// maxHistoryMessages 取数与配置的硬上限（窗口配置值夹在 1..maxHistoryMessages）
	maxHistoryMessages = 20
	// historyTokenBudget 历史段的 token 预算硬顶（估算口径见 estimateTokens）
	historyTokenBudget = 2000
	// maxUserContentTokens 当前轮用户输入的上界 —— **防输入炸弹的工程兜底**（非产品参数）：
	// 正常消息远达不到；超出按 rune 边界截断并记 WARN，不静默丢弃、不改写内容。
	maxUserContentTokens = 4000
)

// estimateTokens 粗略 token 估算：中文 1 字 ≈ 1 token（按 rune 计数）。
//
// 刻意不引 tokenizer：plan §3.3 要求"不追求精确，只需防爆"；引库会把依赖与
// 分词歧义一起引进来，而这里只需要一个单调、可复现的上界估计。
func estimateTokens(s string) int { return utf8.RuneCountInString(s) }

// historyMessages 取同会话历史（**不含当前轮**），时间正序，已按窗口 + 预算裁剪。
//
// 纪律（与人格/情绪段同款）：**历史是增强不是依赖** ——
// 无来源 / 无会话 / 取数报错 → 返回 nil（回落"仅 system + 当前 user"的无记忆形态），不阻断对话。
func (h *AIStreamHandler) historyMessages(in assembleInput) []downstream.Message {
	if h.files == nil || in.conversationID <= 0 {
		return nil
	}
	// 多取一条（maxHistoryMessages+1）：用于识别"最新一条即当前轮已落库"的重复
	msgs, _, err := h.files.ListMessages(in.ctx, in.conversationID, maxHistoryMessages+1, 0)
	if err != nil {
		slog.ErrorContext(in.ctx, "ai-stream list messages for history failed", "err", err)
		return nil
	}

	current := strings.TrimSpace(in.userContent)
	hist := make([]downstream.Message, 0, len(msgs))
	for _, m := range msgs {
		// 文件类消息由 Files 通道单独注入（llm-service 抽取后并入当前 user 消息尾部），
		// 不把对象 URL/正文塞进对话历史（否则"历史"里会出现一堆 MinIO 链接）。
		if m.ContentType == "file" {
			continue
		}
		// 只保留对话角色：system 不由历史产生；tool/其它一律不进（未知角色会污染对话格式）
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		hist = append(hist, downstream.Message{Role: m.Role, Content: content})
	}

	// 当前轮去重：前端流程是"先把 user 消息落库（chat-svc），再打 /ai/stream"，
	// 因此会话最新一条**通常就是当前轮** —— 若原样注入，当前问题会在 prompt 里出现两次。
	// 用内容相等判定（不靠"最新的就是当前轮"这种位置假设，避免调用方未落库时误删历史）。
	if n := len(hist); n > 0 && current != "" {
		if last := hist[n-1]; last.Role == "user" && last.Content == current {
			hist = hist[:n-1]
		}
	}

	return trimHistory(hist)
}

// trimHistory 先按窗口取最近 defaultHistoryMessages 条，再按 token 预算从**最旧端**裁剪。
//
// 保底：**至少保留最近 1 轮（末尾 2 条）** —— 即使单条就超预算也不断当前上下文
// （plan §3.2："保证至少保留最近 1 轮，即使超预算也不断当前上下文"）。
func trimHistory(hist []downstream.Message) []downstream.Message {
	if len(hist) > defaultHistoryMessages {
		hist = hist[len(hist)-defaultHistoryMessages:]
	}
	total := 0
	for _, m := range hist {
		total += estimateTokens(m.Content)
	}
	for len(hist) > 2 && total > historyTokenBudget {
		total -= estimateTokens(hist[0].Content)
		hist = hist[1:]
	}
	return hist
}

// clampUserContent 当前轮用户输入的**上界**保护（防超长输入打爆上游/成本）。
//
// 未超上界时**逐字透传**（含首尾空白 —— 不能顺手"清洗"用户原话）；
// 超出则按 rune 边界截断（不切裂 UTF-8）并记 WARN。
func clampUserContent(ctx context.Context, s string) string {
	n := utf8.RuneCountInString(s)
	if n <= maxUserContentTokens {
		return s
	}
	slog.WarnContext(ctx, "ai-stream user content truncated",
		"orig_runes", n, "limit", maxUserContentTokens)
	return string([]rune(s)[:maxUserContentTokens])
}
