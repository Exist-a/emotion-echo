// Package handler — memory_source.go
//
// CL-08a · A4：记忆摘要的 handler 侧取数（安全降级）。
//
// 边界（与 CL-D-07 同构）：取不到 / 报错 / 为空 ⇒ 注入侧传空串，
// 对话照常进行（**记忆是增强不是依赖**，不编造）。
package handler

import (
	"context"
	"log/slog"

	"emotion-echo-web-bff/internal/downstream"
)

// loadMemorySummary handler 侧的安全取数：任何失败 ⇒ ""（并记 warn，不阻断对话）。
func (h *AIStreamHandler) loadMemorySummary(ctx context.Context) string {
	if h.memory == nil {
		return ""
	}
	userID, ok := downstream.UserIDFromContext(ctx)
	if !ok || userID <= 0 {
		return ""
	}
	summary, err := h.memory.LatestMemorySummary(ctx, userID)
	if err != nil {
		slog.WarnContext(ctx, "ai-stream load memory summary failed, injecting empty", "err", err)
		return ""
	}
	return summary
}
