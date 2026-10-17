// Package logic — sendmessagelogic_agent_test.go
//
// CL-06 Phase 3b：消息级**角色快照**（`agent_id`）的持久化与回读契约。
//
// 语义：
//   - `req.AgentId > 0` ⇒ 写入该消息的 `agent_id`（快照：角色后来改名/删除不影响历史）；
//   - `req.AgentId <= 0` ⇒ 写 **NULL**（"未记录"），不得写 0 —— 0 在本仓语义里恒为"无"。
package logic

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendMessage_WithAgent_RecordsSnapshotAndEchoesIt(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "会话")

	resp, err := NewSendMessageLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		SendMessage(&types.SendMessageReq{Id: id, Role: "assistant", Content: "我在", AgentId: 42})
	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.Message.AgentId, "响应必须回带角色 id")

	// 落库侧：读回来仍是 42（快照）
	msgs, _, err := repo.ListMessagesPage(context.Background(), id, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.NotNil(t, msgs[0].AgentID, "agent_id 必须落库（非 NULL）")
	assert.Equal(t, int64(42), *msgs[0].AgentID)
}

func TestSendMessage_WithoutAgent_StoresNullNotZero(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "会话")

	resp, err := NewSendMessageLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		SendMessage(&types.SendMessageReq{Id: id, Role: "user", Content: "在吗"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Message.AgentId, "未记录时对外视图为 0")

	msgs, _, err := repo.ListMessagesPage(context.Background(), id, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Nil(t, msgs[0].AgentID, "未传角色时必须存 NULL（不能写 0 —— 0 与'无'同义）")
}

func TestSendMessage_NegativeAgent_TreatedAsUnrecorded(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "会话")

	_, err := NewSendMessageLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		SendMessage(&types.SendMessageReq{Id: id, Role: "assistant", Content: "x", AgentId: -3})
	require.NoError(t, err)

	msgs, _, err := repo.ListMessagesPage(context.Background(), id, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Nil(t, msgs[0].AgentID, "负数角色 id 视为未记录（不落库）")
}

// 列表路径也要回带（前端历史标注"这条是谁回的"靠它）。
func TestListMessages_ReturnsAgentSnapshot(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "会话")

	_, err := NewSendMessageLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		SendMessage(&types.SendMessageReq{Id: id, Role: "assistant", Content: "first", AgentId: 7})
	require.NoError(t, err)
	_, err = NewSendMessageLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		SendMessage(&types.SendMessageReq{Id: id, Role: "assistant", Content: "second", AgentId: 8})
	require.NoError(t, err)

	out, err := NewListMessagesLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		ListMessages(&types.ListMessagesReq{Id: id, Limit: 10})
	require.NoError(t, err)
	require.Len(t, out.Messages, 2)
	assert.Equal(t, int64(7), out.Messages[0].AgentId, "历史消息必须带回当时的角色（快照语义）")
	assert.Equal(t, int64(8), out.Messages[1].AgentId)
}
