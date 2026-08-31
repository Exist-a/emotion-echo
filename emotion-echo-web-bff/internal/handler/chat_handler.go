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
}

// NewChatHandler 构造
func NewChatHandler(chat downstream.ChatClient) *ChatHandler {
	return &ChatHandler{chat: chat}
}

// Register 注册路由
func (h *ChatHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/conversations", h.listConversations)
	r.POST("/api/v1/conversations", h.createConversation)
	r.POST("/api/v1/conversations/:id/messages", h.sendMessage)
	r.GET("/api/v1/conversations/:id/messages", h.listMessages)
	r.DELETE("/api/v1/conversations/:id", h.deleteConversation)
}

// listConversations 会话列表（前端契约 {list, hasMore}）
//
// 注：chat-svc 暂无 list 端点，BFF 先返回空列表（前端首次加载不 404）。
// 待 chat-svc 增加 GET /conversations 后透传。
func (h *ChatHandler) listConversations(c *gin.Context) {
	OK(c, gin.H{"list": []ConversationItemVM{}, "hasMore": false})
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
