// Package handler — memory_handler.go
//
// CL-08a · A5：「我的记忆」页的 BFF 端点。
//
//	POST /api/v1/memory/notes   { content }  → 写入自述 + 固定回执（不调大模型回答）
//	GET  /api/v1/memory/graph                → 记忆图（PG 事实渲染）
//
// 安全：user_id 一律取自鉴权上下文（网关注入），**不接受客户端传**（同 CL-D-30 口径）。
package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// memoryNoteMaxRunes 与 ai-svc 侧保持一致（BFF 先挡一层，避免明显超长的请求打到下游）。
const memoryNoteMaxRunes = 4000

// MemoryHandler 「我的记忆」端点。
type MemoryHandler struct {
	client downstream.MemoryClient
}

func NewMemoryHandler(client downstream.MemoryClient) *MemoryHandler {
	return &MemoryHandler{client: client}
}

// CreateNote POST /api/v1/memory/notes
func (h *MemoryHandler) CreateNote(c *gin.Context) {
	if h.client == nil {
		Fail(c, http.StatusServiceUnavailable, 503, "memory service unavailable")
		return
	}
	ctx := session.WithRequestAuth(c)
	userID, ok := downstream.UserIDFromContext(ctx)
	if !ok || userID <= 0 {
		Fail(c, http.StatusUnauthorized, 401, "unauthorized")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		Fail(c, http.StatusBadRequest, 1, "validation: content is required")
		return
	}
	if utf8.RuneCountInString(content) > memoryNoteMaxRunes {
		Fail(c, http.StatusBadRequest, 1, "validation: content too long")
		return
	}

	res, err := h.client.CreateNote(ctx, userID, content)
	if err != nil {
		Fail(c, http.StatusBadGateway, 502, err.Error())
		return
	}
	OK(c, res)
}

// GetGraph GET /api/v1/memory/graph
func (h *MemoryHandler) GetGraph(c *gin.Context) {
	if h.client == nil {
		Fail(c, http.StatusServiceUnavailable, 503, "memory service unavailable")
		return
	}
	ctx := session.WithRequestAuth(c)
	userID, ok := downstream.UserIDFromContext(ctx)
	if !ok || userID <= 0 {
		Fail(c, http.StatusUnauthorized, 401, "unauthorized")
		return
	}

	graph, err := h.client.GetGraph(ctx, userID)
	if err != nil {
		Fail(c, http.StatusBadGateway, 502, err.Error())
		return
	}
	OK(c, graph)
}
