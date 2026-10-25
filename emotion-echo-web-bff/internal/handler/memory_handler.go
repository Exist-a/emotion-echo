// Package handler — memory_handler.go
//
// CL-08a · A5：「我的记忆」页的 BFF 端点。
//
//	POST   /api/v1/memory/notes     { content } → 写入自述 + 固定回执（不调大模型回答）
//	GET    /api/v1/memory/graph                 → 记忆图（PG 事实渲染）
//	GET    /api/v1/memory/items                 → 记忆列表（CL-08d 管理页删除的列举面）
//	DELETE /api/v1/memory/facts/:id             → 删单条记忆（闭环：PG→图→摘要）
//	DELETE /api/v1/memory/notes/:id             → 删一条自述（级联其事实与情节）
//	DELETE /api/v1/memory                       → 删除全部记忆
//
// 安全：user_id 一律取自鉴权上下文（网关注入），**不接受客户端传**（同 CL-D-30 口径）。
package handler

import (
	"context"
	"net/http"
	"strconv"
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

// authUserID 统一取可信身份（CL-08d 复用）。
func (h *MemoryHandler) authUserID(c *gin.Context) (ctx0 context.Context, userID int64, ok bool) {
	ctx0 = session.WithRequestAuth(c)
	userID, ok = downstream.UserIDFromContext(ctx0)
	if !ok || userID <= 0 {
		Fail(c, http.StatusUnauthorized, 401, "unauthorized")
	}
	return ctx0, userID, ok
}

// ListMemory GET /api/v1/memory/items
func (h *MemoryHandler) ListMemory(c *gin.Context) {
	if h.client == nil {
		Fail(c, http.StatusServiceUnavailable, 503, "memory service unavailable")
		return
	}
	ctx, userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	items, err := h.client.ListMemory(ctx, userID)
	if err != nil {
		Fail(c, http.StatusBadGateway, 502, "list memory failed")
		return
	}
	OK(c, gin.H{"items": items})
}

// DeleteMemory DELETE /api/v1/memory/facts/:id | notes/:id | (all)
func (h *MemoryHandler) DeleteMemory(c *gin.Context, scope string) {
	if h.client == nil {
		Fail(c, http.StatusServiceUnavailable, 503, "memory service unavailable")
		return
	}
	ctx, userID, ok := h.authUserID(c)
	if !ok {
		return
	}
	var id int64
	if scope != "all" {
		id, ok = parseIDParam(c)
		if !ok {
			return
		}
	}
	note, err := h.client.DeleteMemory(ctx, userID, scope, id)
	if err != nil {
		Fail(c, http.StatusBadGateway, 502, "delete memory failed")
		return
	}
	OK(c, gin.H{"ok": true, "note": note})
}

// parseIDParam 解析 :id（正整数）。
func parseIDParam(c *gin.Context) (int64, bool) {
	raw := c.Param("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid id")
		return 0, false
	}
	return id, true
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
