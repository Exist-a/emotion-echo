// Package logic — deleteconversationlogic_test.go
//
// Stage 30-B RED: DeleteConversationLogic 测试。
//
// 会话删除是 conversation.closed 事件的唯一生产者（chat-svc 此前
// 定义了该事件类型但从未发布）。流程：鉴权 → 会话存在 + owner 校验
// → repo 删除（会话 + 消息）→ 发布 conversation.closed（best-effort）。
package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedConv 在 InMemory repo 里建一个属于 uid 的会话
func seedConv(t *testing.T, repo *repository.InMemoryConversationRepo, id, uid int64) {
	t.Helper()
	conv := &model.Conversation{
		ID:        id,
		UserID:    uid,
		Title:     "t",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, repo.CreateConversation(context.Background(), conv))
}

func TestDeleteConversationLogic_HappyPath_DeletesAndPublishesClosed(t *testing.T) {
	svcCtx, repo, pub := newTestCtx(t)
	seedConv(t, repo, 1, 100)

	l := NewDeleteConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.DeleteConversation(&types.DeleteConversationReq{Id: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)

	// repo 中已删除
	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got, "会话应已从 repo 删除")

	// 发布了 conversation.closed（唯一事件）
	evts := pub.Events(events.TopicChatEvents)
	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeConversationClosed, evts[0].Type)
	assert.Equal(t, "chat-svc", evts[0].Source)
	data, ok := evts[0].Data.(events.ConversationClosedData)
	require.True(t, ok, "Data 应为 ConversationClosedData")
	assert.Equal(t, int64(1), data.ConversationID)
	assert.Equal(t, int64(100), data.UserID)
	assert.NotZero(t, data.ClosedAt, "ClosedAt 应填充")
}

func TestDeleteConversationLogic_NotOwner_Forbidden(t *testing.T) {
	svcCtx, repo, pub := newTestCtx(t)
	seedConv(t, repo, 1, 100)

	l := NewDeleteConversationLogic(ctxWithUserID(context.Background(), 999), svcCtx)
	resp, err := l.DeleteConversation(&types.DeleteConversationReq{Id: 1})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
	assert.Empty(t, pub.Events(events.TopicChatEvents), "非 owner 不应发事件")
}

func TestDeleteConversationLogic_NotFound_ErrNotFound(t *testing.T) {
	svcCtx, _, pub := newTestCtx(t)

	l := NewDeleteConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.DeleteConversation(&types.DeleteConversationReq{Id: 999})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrNotFound)
	assert.Empty(t, pub.Events(events.TopicChatEvents))
}

func TestDeleteConversationLogic_NoUserID_Unauthorized(t *testing.T) {
	svcCtx, _, _ := newTestCtx(t)

	l := NewDeleteConversationLogic(context.Background(), svcCtx)
	resp, err := l.DeleteConversation(&types.DeleteConversationReq{Id: 1})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestDeleteConversationLogic_RepoError_Propagated(t *testing.T) {
	svcCtx, _, _ := newTestCtx(t)
	seedConv(t, svcCtx.ConversationRepo.(*repository.InMemoryConversationRepo), 1, 100)
	// 用 fail-deleting repo 包装：DeleteConversation 返错
	svcCtx.ConversationRepo = &failingDeleteRepo{inner: svcCtx.ConversationRepo}

	l := NewDeleteConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.DeleteConversation(&types.DeleteConversationReq{Id: 1})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

// failingDeleteRepo 包装 ConversationRepo，仅让 DeleteConversation 失败
type failingDeleteRepo struct {
	inner repository.ConversationRepo
}

func (f *failingDeleteRepo) CreateConversation(ctx context.Context, c *model.Conversation) error {
	return f.inner.CreateConversation(ctx, c)
}
func (f *failingDeleteRepo) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	return f.inner.GetConversationByID(ctx, id)
}
func (f *failingDeleteRepo) IncrementMessageCount(ctx context.Context, conversationID int64) error {
	return f.inner.IncrementMessageCount(ctx, conversationID)
}
func (f *failingDeleteRepo) AppendMessage(ctx context.Context, m *model.Message) error {
	return f.inner.AppendMessage(ctx, m)
}
func (f *failingDeleteRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error) {
	return f.inner.ListMessages(ctx, conversationID, limit)
}
func (f *failingDeleteRepo) DeleteConversation(ctx context.Context, id int64) error {
	return errors.New("delete failed")
}
func (f *failingDeleteRepo) Ping(ctx context.Context) error { return f.inner.Ping(ctx) }
