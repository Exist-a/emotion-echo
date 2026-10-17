// Package logic — listconversationslogic_agent_test.go
//
// CL-06 · Phase 8 运行态验收抓出的**真缺陷**回归钉：
//
// 现象（经网关实测）：`PATCH /conversations/:id {agentId:3}` 后，DB 里
// `conversations.agent_id = 3`（写对了），但 `GET /conversations` 列表回的 `agentId` 恒为 **0**。
//
// 根因：**视图投影点漏字段** —— `types.ConversationView` 有了 `AgentId`、
// proto 映射 `toProtoConversation` 也带上了它，但 `ListConversationsLogic` /
// `CreateConversationLogic` 里手工构造 `ConversationView{...}` 的那两处**没有赋 AgentId**。
// 这类"字段加了、投影点没跟"的缺陷单元测试抓不到（各层单测都只测自己那一段），
// 只有"写→读回"的端到端断言能抓（本次即由 `phase8_api_check.py` 的 T-6c 抓到）。
//
// 本文件把"每个投影点都必须带 agent_id"钉死。
package logic

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListConversations_ViewCarriesAgentID(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	ctx := ctxWithUserID(context.Background(), 1)

	id := newConv(t, repo, 1, "验收会话")
	require.NoError(t, repo.UpdateAgentID(context.Background(), id, 3))

	resp, err := NewListConversationsLogic(ctx, svcCtx).ListConversations(&types.ListConversationsReq{Limit: 20})
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, int64(3), resp.List[0].AgentId,
		"列表视图必须回带 agent_id（漏了 ⇒ 前端切换器/消息标注全显示默认角色）")
}

// 未设置角色的会话 ⇒ 0（前端据此回落默认角色；不是错误）
func TestListConversations_ViewAgentIDZeroWhenUnset(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	ctx := ctxWithUserID(context.Background(), 1)

	newConv(t, repo, 1, "无角色会话")

	resp, err := NewListConversationsLogic(ctx, svcCtx).ListConversations(&types.ListConversationsReq{Limit: 20})
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, int64(0), resp.List[0].AgentId)
}

// 建会话的响应视图同样必须带 agent_id（同一个投影纪律；此处新建时应为 0）
func TestCreateConversation_ViewCarriesAgentID(t *testing.T) {
	t.Parallel()
	svcCtx, _, _ := newTestCtx(t)
	ctx := ctxWithUserID(context.Background(), 1)

	resp, err := NewCreateConversationLogic(ctx, svcCtx).CreateConversation(&types.CreateConversationReq{Title: "t"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(0), resp.Conversation.AgentId,
		"新建会话未指定角色 ⇒ 0（前端回落默认角色）")
}
