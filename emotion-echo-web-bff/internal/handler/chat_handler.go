// Package handler — chat_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.38-41: chat handler（BFF → chat-svc）
//
// 端点：
//
//	POST   /api/v1/conversations              → {conversation}
//	POST   /api/v1/conversations/:id/messages → {message}
//	GET    /api/v1/conversations/:id/messages?limit= → {messages}
//	DELETE /api/v1/conversations/:id          → {success, id}
//
// 注：PinConversation 下游未实现，不暴露路由（接口保留在 ChatClient）。
package handler

import (
	"context"
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
	// agents（CL-06）：**仅用于会话切角色的归属校验**。
	// chat-svc 访问不到用户角色表 ⇒ "这个 agent_id 是不是你的"只能在 BFF 判（见 updateConversation）。
	// nil = 未装配 ⇒ 带 agentId 的请求 503（fail-closed，不是跳过校验）。
	agents downstream.AgentProfileClient
}

// NewChatHandler 构造
func NewChatHandler(chat downstream.ChatClient) *ChatHandler {
	return &ChatHandler{chat: chat}
}

// NewChatHandlerWithIntent 构造带意图分类的 handler（Stage 82 PR-3b）
func NewChatHandlerWithIntent(chat downstream.ChatClient, ic downstream.LLMIntentClassifier) *ChatHandler {
	return &ChatHandler{chat: chat, intentClassifier: ic}
}

// WithAgentProfiles 注入角色客户端（CL-06 会话切角色的归属校验用）
func (h *ChatHandler) WithAgentProfiles(a downstream.AgentProfileClient) *ChatHandler {
	h.agents = a
	return h
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

// updateConversation PATCH /api/v1/conversations/:id
//
// Stage 72：改标题。**CL-06 起同一个端点兼"切角色"**（chat-svc 的
// UpdateConversationRequest 两字段各自可选：title 改名 / agent_id 切角色）。
//
// 归属校验在此处（不是 chat-svc）：chat-svc 访问不到用户角色表，而"这个 agent_id 是不是
// 调用者的"是安全判定 ⇒ **BFF 查一次自己的角色列表**（顺带触发默认角色物化）。
// 判不过就**不下达下游**（否则会话会被切到别人的角色 id 上 = 跨用户 IDOR）。
func (h *ChatHandler) updateConversation(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req struct {
		Title   string `json:"title"`
		AgentID int64  `json:"agentId"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	ctx := session.WithRequestAuth(c)
	if req.AgentID != 0 {
		if status, code, msg := h.checkAgentOwnership(ctx, req.AgentID); code != 0 {
			Fail(c, status, code, msg)
			return
		}
	}
	resp, err := h.chat.UpdateConversation(ctx, id, req.Title, req.AgentID)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"success": resp.Success, "id": resp.Id, "title": resp.Title, "agentId": resp.AgentID})
}

// checkAgentOwnership 校验 agentID 属于当前用户。
//
// 返回 (httpStatus, code, message)；code == 0 表示通过。
// 语义选择：**不属于自己 ⇒ 400 而不是 403** —— 403 会透露"该 id 存在但不属于你"，
// 400 + "not found for current user" 对越权与不存在**同形**，不泄漏存在性。
// 查不动上游（user-svc 故障）⇒ 502：既不放行也不归罪于用户请求。
func (h *ChatHandler) checkAgentOwnership(ctx context.Context, agentID int64) (int, int, string) {
	if h.agents == nil {
		return http.StatusServiceUnavailable, 1, "agent profile service unavailable"
	}
	if agentID < 0 {
		return http.StatusBadRequest, 1, "validation: invalid agent id"
	}
	profiles, err := h.agents.ListAgentProfiles(ctx)
	if err != nil {
		return statusFor(err), 1, err.Error()
	}
	for i := range profiles {
		if profiles[i].ID == agentID {
			return 0, 0, ""
		}
	}
	return http.StatusBadRequest, 1, "validation: agent profile not found for current user"
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
	// CL-05 Phase 5：游标翻页。`cursor` 是 chat-svc 编码的**不透明**游标
	// （复合键 `<unixMicros>:<id>`），BFF 只透传，不解析。
	cursor := c.Query("cursor")
	page, err := h.chat.ListConversations(session.WithRequestAuth(c), limit, offset, cursor)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	items := make([]ConversationItemVM, 0, len(page.List))
	for i := range page.List {
		items = append(items, toConversationItemVM(&page.List[i]))
	}
	// 响应键 `cursor` 与消息列表端点（listMessages）同形：它是**下一页游标**
	// （空 = 没有更多），不是回显请求值。
	OK(c, gin.H{"list": items, "hasMore": page.HasMore, "cursor": page.NextCursor})
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
	// CL-06：**用户消息不承载角色** ⇒ 忽略客户端传来的 agentId。
	//
	// 为什么必须显式清掉（而不是"反正前端不传"）：
	//   · 角色快照的语义是"这条 AI 回复由谁产生"，只应出现在 assistant 消息上；
	//   · 该字段由客户端直传、此处**没有归属校验**（切角色那条路径有），
	//     留着等于开了一条"把任意 agent_id 写进消息"的旁路（第二方核对 2026-10-17 指出）；
	//   · AI 回复的角色戳走的是 ai/stream → saveAIMessage（那里经 resolveAgentProfile 校验）。
	req.AgentID = 0
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
	// 2026-10-13：游标分页（cursor = 上一页最早一条的 id；0/缺省 = 从最新开始）
	var cursor int64
	if v := c.Query("cursor"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cursor = n
		}
	}
	msgs, hasMore, err := h.chat.ListMessages(session.WithRequestAuth(c), id, limit, cursor)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	items := make([]MessageItemVM, 0, len(msgs))
	for i := range msgs {
		items = append(items, toMessageItemVM(&msgs[i]))
	}
	// nextCursor = 本页最早一条的 id（消息已按时间升序返回，第一条即最早），
	// 供前端上翻加载更旧一页；空页时回传请求的 cursor。
	nextCursor := cursor
	if len(msgs) > 0 {
		nextCursor = msgs[0].ID
	}
	OK(c, gin.H{"list": items, "cursor": nextCursor, "hasMore": hasMore})
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
