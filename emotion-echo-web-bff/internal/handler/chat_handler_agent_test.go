// Package handler — chat_handler_agent_test.go
//
// CL-06 · Phase 4：**会话切角色**的 BFF 契约（T-6 的 BFF 侧）＋ 角色字段向前的 VM 透传。
//
// 安全边界（本文件的主要被验对象）：**归属校验在 BFF**。
// chat-svc 无法访问用户角色表（跨库无外键、也不该反向依赖 user-svc），
// 故"这个 agent_id 是不是你的"必须在 BFF 判 —— 否则任意登录用户可以把会话切到
// 别人的角色 id 上，用别人的人设与音色（**跨用户 IDOR**）。
//
// fail-closed：未装配角色客户端时，带 agentId 的切换请求一律拒绝（不是"跳过校验"）。
package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAgentClient 实现 downstream.AgentProfileClient（只读 + 记录写入）
type fakeAgentClient struct {
	profiles   []downstream.AgentProfile
	listErr    error
	upsertResp *downstream.AgentProfile
	upsertErr  error
	deleteErr  error

	gotList    int
	gotUpserts []downstream.AgentProfileUpsertReq
	gotDeletes []int64
}

func (f *fakeAgentClient) ListAgentProfiles(_ context.Context) ([]downstream.AgentProfile, error) {
	f.gotList++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.profiles, nil
}

func (f *fakeAgentClient) UpsertAgentProfile(_ context.Context, req downstream.AgentProfileUpsertReq) (*downstream.AgentProfile, error) {
	f.gotUpserts = append(f.gotUpserts, req)
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	if f.upsertResp != nil {
		return f.upsertResp, nil
	}
	return &downstream.AgentProfile{ID: 1, Name: req.Name, Persona: req.Persona, Voice: req.Voice}, nil
}

func (f *fakeAgentClient) DeleteAgentProfile(_ context.Context, agentID int64) error {
	f.gotDeletes = append(f.gotDeletes, agentID)
	return f.deleteErr
}

// newChatRouterWithAgents 起一个同时挂了 chat + 角色客户端的路由
func newChatRouterWithAgents(chat downstream.ChatClient, agents downstream.AgentProfileClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&ChatHandler{chat: chat, agents: agents}).Register(r)
	return r
}

func patchConversation(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/5", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─────────── 会话切角色 ───────────

func TestUpdateConversation_SwitchAgent_OwnedAgent_PassesAgentID(t *testing.T) {
	fc := &fakeChatClient{}
	agents := &fakeAgentClient{profiles: []downstream.AgentProfile{
		{ID: 7, Name: "小E", IsDefault: true},
		{ID: 9, Name: "阿哲"},
	}}
	r := newChatRouterWithAgents(fc, agents)

	w := patchConversation(t, r, `{"agentId":9}`)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, int64(9), fc.gotAgentID, "生效角色必须透传给 chat-svc")
	assert.Empty(t, fc.gotTitle, "只切角色 ⇒ title 传空（不动标题）")
	assert.Equal(t, 1, agents.gotList, "必须查一次归属")
	assert.Contains(t, w.Body.String(), `"agentId":9`, "响应回显生效后的角色，前端据此更新本地态")
}

func TestUpdateConversation_SwitchAgent_NotOwned_Rejected(t *testing.T) {
	fc := &fakeChatClient{}
	// 只拥有 7；请求切到 9 ⇒ 越权
	agents := &fakeAgentClient{profiles: []downstream.AgentProfile{{ID: 7, Name: "小E", IsDefault: true}}}
	r := newChatRouterWithAgents(fc, agents)

	w := patchConversation(t, r, `{"agentId":9}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, int64(0), fc.gotConvID, "越权请求**不得**下达 chat-svc（必须在 BFF 拦下）")
}

func TestUpdateConversation_SwitchAgent_NoAgentClient_FailClosed(t *testing.T) {
	fc := &fakeChatClient{}
	r := newChatRouterWithAgents(fc, nil) // 未装配角色客户端

	w := patchConversation(t, r, `{"agentId":9}`)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "无法校验归属 ⇒ 拒绝，不得放行")
	assert.Equal(t, int64(0), fc.gotConvID)
}

func TestUpdateConversation_SwitchAgent_ListUpstreamError_Returns502(t *testing.T) {
	fc := &fakeChatClient{}
	agents := &fakeAgentClient{listErr: errors.New("user-svc down")}
	r := newChatRouterWithAgents(fc, agents)

	w := patchConversation(t, r, `{"agentId":9}`)

	assert.Equal(t, http.StatusBadGateway, w.Code, "查不动归属 ⇒ 502（不是放行、也不是 400 赖用户）")
	assert.Equal(t, int64(0), fc.gotConvID)
}

// 零回归：只改标题的既有路径**不得**因为新校验而需要角色客户端／不得多发一次下游调用
func TestUpdateConversation_TitleOnly_NoAgentLookup(t *testing.T) {
	fc := &fakeChatClient{}
	agents := &fakeAgentClient{}
	r := newChatRouterWithAgents(fc, agents)

	w := patchConversation(t, r, `{"title":"今晚咨询"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "今晚咨询", fc.gotTitle)
	assert.Equal(t, int64(0), fc.gotAgentID, "只改标题 ⇒ 不改角色（0 = 不动）")
	assert.Equal(t, 0, agents.gotList, "不得为改标题而多查一次角色列表")
}

// ─────────── 角色字段的向前透传（Phase 7 切换器/消息标注要用） ───────────

func TestListConversations_VM_CarriesAgentID(t *testing.T) {
	fc := &fakeChatClient{convs: []downstream.ConversationView{{ID: 5, Title: "t", AgentID: 9}}}
	r := newChatRouterWithAgents(fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"agentId":9`, "会话列表必须带 agentId（切换器显示当前角色）")
}

func TestListMessages_VM_CarriesAgentID(t *testing.T) {
	fc := &fakeChatClient{messages: []downstream.MessageView{
		{ID: 1, ConversationID: 5, Role: "user", Content: "hi"},
		{ID: 2, ConversationID: 5, Role: "assistant", Content: "在", AgentID: 9},
	}}
	r := newChatRouterWithAgents(fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/5/messages", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"agentId":9`, "AI 消息必须带 agentId（历史标注'这条是谁回的'）")
}

// CL-06 · 第二方核对（2026-10-17）指出：非流式发消息路径把客户端的 agentId **原样透传**，
// 且此处**没有归属校验**（切角色那条路径才有）⇒ 等于一条"把任意 agent_id 写进消息"的旁路。
// 语义上用户消息本就不该带角色（快照是"这条 AI 回复由谁产生"）⇒ 显式清零。
func TestSendMessage_IgnoresClientSuppliedAgentID(t *testing.T) {
	fc := &fakeChatClient{msg: &downstream.MessageView{ID: 10, ConversationID: 5, Content: "hi"}}
	r := newChatRouterWithAgents(fc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/5/messages",
		bytes.NewReader([]byte(`{"content":"你好","agentId":999999}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(0), fc.gotSendReq.AgentID,
		"用户消息不得带角色戳（既无归属校验、语义上也不该有）")
}
