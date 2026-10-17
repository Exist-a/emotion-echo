// Package logic — updateconversationlogic_agent_test.go
//
// CL-06 Phase 3b：`UpdateConversation` 支持**会话级角色切换**（`agent_id`）的契约测试。
//
// 语义要点（先写测试）：
//   - `agent_id > 0` ⇒ 设置本会话当前角色；`0` ⇒ **本次不改**（后端不区分"设为默认"与"不动"，
//     "用默认角色"由 NULL/0 表达 —— 见迁移 c012 与 proto Conversation.agent_id 注释）。
//   - `title` 与 `agent_id` **可单传、可同传**；**两者都空 ⇒ 校验失败**（不再沿用"title 必填"，
//     否则切换角色会被标题校验挡住）。
//   - 向后兼容（**零回归**）：老调用方只传 `title` ⇒ 标题照改、**agent_id 不受影响**。
//   - 属主不符 ⇒ forbidden；会话不存在 ⇒ repository.ErrNotFound。
package logic

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newConv 造一条属于 uid 的会话，返回其 id。
func newConv(t *testing.T, repo *repository.InMemoryConversationRepo, uid int64, title string) int64 {
	t.Helper()
	c := &model.Conversation{UserID: uid, Title: title, Status: 1}
	require.NoError(t, repo.CreateConversationTx(nil, context.Background(), c))
	require.NotZero(t, c.ID, "InMemory 仓储应当回填自增 ID")
	return c.ID
}

func getConv(t *testing.T, repo *repository.InMemoryConversationRepo, id int64) *model.Conversation {
	t.Helper()
	c, err := repo.GetConversationByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, c)
	return c
}

func TestUpdateConversation_AgentOnly_SetsAgent(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "原标题")

	resp, err := NewUpdateConversationLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, AgentId: 42})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, int64(42), resp.AgentId, "响应应回显生效后的角色 id")

	got := getConv(t, repo, id)
	require.NotNil(t, got.AgentID, "agent_id 应被写入")
	assert.Equal(t, int64(42), *got.AgentID)
	assert.Equal(t, "原标题", got.Title, "只切角色时标题不得被动")
}

func TestUpdateConversation_TitleAndAgent_BothApply(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "原标题")

	_, err := NewUpdateConversationLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, Title: "新标题", AgentId: 7})
	require.NoError(t, err)

	got := getConv(t, repo, id)
	assert.Equal(t, "新标题", got.Title)
	require.NotNil(t, got.AgentID)
	assert.Equal(t, int64(7), *got.AgentID)
}

// 零回归：老调用方（只改标题）不得影响 agent 绑定。
func TestUpdateConversation_TitleOnly_KeepsAgentUntouched(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "原标题")

	// 先绑定一个角色
	_, err := NewUpdateConversationLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, AgentId: 9})
	require.NoError(t, err)

	// 再只改标题
	_, err = NewUpdateConversationLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, Title: "只改标题"})
	require.NoError(t, err)

	got := getConv(t, repo, id)
	assert.Equal(t, "只改标题", got.Title)
	require.NotNil(t, got.AgentID, "只改标题不得清掉已绑定的角色")
	assert.Equal(t, int64(9), *got.AgentID)
}

func TestUpdateConversation_NothingToUpdate_Rejected(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "原标题")

	_, err := NewUpdateConversationLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id})
	require.Error(t, err)
	assert.ErrorContains(t, err, "validation", "两者都空必须报校验错（不能静默成功）")

	got := getConv(t, repo, id)
	assert.Equal(t, "原标题", got.Title, "被拒的请求不得改动任何字段")
	assert.Nil(t, got.AgentID)
}

func TestUpdateConversation_NegativeAgentID_Rejected(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	const uid = int64(100)
	id := newConv(t, repo, uid, "原标题")

	_, err := NewUpdateConversationLogic(ctxWithUserID(context.Background(), uid), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, AgentId: -1})
	require.Error(t, err)
	assert.ErrorContains(t, err, "validation", "负数角色 id 必须被拒")
	assert.Nil(t, getConv(t, repo, id).AgentID)
}

func TestUpdateConversation_OtherUsersConversation_Forbidden(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	id := newConv(t, repo, 100, "别人的会话")

	_, err := NewUpdateConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id + 999, AgentId: 5})
	assert.ErrorIs(t, err, repository.ErrNotFound, "不存在的会话按 NotFound")

	_, err = NewUpdateConversationLogic(ctxWithUserID(context.Background(), 200), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, AgentId: 5})
	require.Error(t, err)
	assert.ErrorContains(t, err, "forbidden", "他人会话不得改（防越权切角色）")

	got := getConv(t, repo, id)
	assert.Nil(t, got.AgentID, "越权请求不得写入")
}

func TestUpdateConversation_Unauthorized_WhenNoUserID(t *testing.T) {
	t.Parallel()
	svcCtx, repo, _ := newTestCtx(t)
	id := newConv(t, repo, 100, "原标题")

	_, err := NewUpdateConversationLogic(context.Background(), svcCtx).
		UpdateConversation(&types.UpdateConversationReq{Id: id, AgentId: 5})
	require.Error(t, err)
	assert.ErrorContains(t, err, "unauthorized")
}
