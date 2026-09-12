// Package handler — chat_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.38-41: chat handler（BFF → chat-svc）
//
// 端点：
//   POST   /api/v1/conversations              → {conversation}
//   POST   /api/v1/conversations/:id/messages → {message}
//   GET    /api/v1/conversations/:id/messages?limit= → {messages}
//   DELETE /api/v1/conversations/:id          → {success, id}
//
// 注：PinConversation 下游未实现，不暴露路由（接口保留在 ChatClient）。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// ChatHandler 处理 /api/v1/conversations/* 端点
type ChatHandler struct {
	chat downstream.ChatClient
	// intentClassifier（Stage 82 PR-3b）：llm-service 意图分类；nil = 降级不标注
	intentClassifier downstream.LLMIntentClassifier
}

// NewChatHandler 构造
func NewChatHandler(chat downstream.ChatClient) *ChatHandler {
	return &ChatHandler{chat: chat}
}

// NewChatHandlerWithIntent 构造带意图分类的 handler（Stage 82 PR-3b）
func NewChatHandlerWithIntent(chat downstream.ChatClient, ic downstream.LLMIntentClassifier) *ChatHandler {
	return &ChatHandler{chat: chat, intentClassifier: ic}
}

// Register 注册路由
func (h *ChatHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/conversations", h.listConversations)
	r.POST("/api/v1/conversations", h.createConversation)
	// Stage 72：PATCH 接真实 chat-svc UpdateConversation RPC（决策 4 ADR §八 收口）
	r.PATCH("/api/v1/conversations/:id", h.updateConversation)
	// Stage 72：置顶/取消置顶（chat-svc PinConversation RPC 已落地）
	r.POST("/api/v1/conversations/:id/pin", h.pinConversation)
	r.POST("/api/v1/conversations/:id/messages", h.sendMessage)
	r.GET("/api/v1/conversations/:id/messages", h.listMessages)
	r.DELETE("/api/v1/conversations/:id", h.deleteConversation)
}

// updateConversation PATCH /api/v1/conversations/:id（Stage 72 真实实现）
//
// 调 chat-svc UpdateConversation RPC（当前仅支持 title）。
func (h *ChatHandler) updateConversation(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	resp, err := h.chat.UpdateConversation(session.WithRequestAuth(c), id, req.Title)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"success": resp.Success, "id": resp.Id, "title": resp.Title})
}

// pinConversation POST /api/v1/conversations/:id/pin（Stage 72 真实实现）
//
// 调 chat-svc PinConversation RPC；请求体 {"isPinned": true|false}。
func (h *ChatHandler) pinConversation(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	// Stage 73 e2e 修正：前端 store togglePinConversation 发 {"isTop": bool}
	//（conversation.ts），字段名必须对齐前端契约，否则状态永远 false。
	var req struct {
		IsTop bool `json:"isTop"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	if err := h.chat.PinConversation(session.WithRequestAuth(c), id, req.IsTop); err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"success": true, "id": id, "isPinned": req.IsTop})
}

// listConversations 会话列表（前端契约 {list, hasMore}）
//
// Stage 36-A2.2：透传 chat-svc GET /api/v1/conversations（用户隔离 + 分页）。
// 用户 ID 由 session.WithRequestAuth 注入 ctx → applyAuthHeader → X-User-Id。
func (h *ChatHandler) listConversations(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	convs, hasMore, err := h.chat.ListConversations(session.WithRequestAuth(c), limit, offset)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	items := make([]ConversationItemVM, 0, len(convs))
	for i := range convs {
		items = append(items, toConversationItemVM(&convs[i]))
	}
	OK(c, gin.H{"list": items, "hasMore": hasMore})
}

func (h *ChatHandler) createConversation(c *gin.Context) {
	var req downstream.CreateConversationReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	conv, err := h.chat.CreateConversation(session.WithRequestAuth(c), req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	// 前端期望 data 直接是 ConversationItem（非 {conversation} 包装）
	OK(c, toConversationItemVM(conv))
}

func (h *ChatHandler) sendMessage(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req downstream.SendMessageReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	// Stage 82 PR-3b：发送前经 llm-service 标注意图（规则式 6 类；
	// 失败/未装配 → intent 空 = 未分类，不阻断发送）
	if h.intentClassifier != nil && req.Content != "" {
		if intent, err := h.intentClassifier.ClassifyIntent(c.Request.Context(), req.Content); err == nil && intent != "" {
			req.Intent = intent
		}
	}
	msg, err := h.chat.SendMessage(session.WithRequestAuth(c), id, req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, toMessageItemVM(msg))
}

func (h *ChatHandler) listMessages(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	msgs, err := h.chat.ListMessages(session.WithRequestAuth(c), id, limit)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	items := make([]MessageItemVM, 0, len(msgs))
	for i := range msgs {
		items = append(items, toMessageItemVM(&msgs[i]))
	}
	OK(c, gin.H{"list": items})
}

func (h *ChatHandler) deleteConversation(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.chat.DeleteConversation(session.WithRequestAuth(c), id); err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"success": true, "id": id})
}

// pathID 解析 path 参数 :id 为 int64；非法时写 400 并返回 false
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid id")
		return 0, false
	}
	return id, true
}
