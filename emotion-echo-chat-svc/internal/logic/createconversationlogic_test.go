package logic

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/middleware"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ctxWithUserID + newTestCtx are shared helpers for all logic/*_test.go
// files in this package. Defined here (the older sibling) so existing
// imports keep working; sendmessagelogic_test.go reuses them.
func ctxWithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, middleware.CtxUserIDKey{}, uid)
}

// 测试公用：构造一个完整测试上下文
func newTestCtx(t *testing.T) (*svc.ServiceContext, *repository.InMemoryConversationRepo, *events.InMemoryEventPublisher) {
	t.Helper()
	repo := repository.NewInMemoryConversationRepo()
	pub := events.NewInMemoryEventPublisher()
	svcCtx := &svc.ServiceContext{
		ConversationRepo: repo,
		EventPublisher:   pub,
	}
	return svcCtx, repo, pub
}

func TestCreateConversationLogic_WithTitle_PublishesEvent(t *testing.T) {
	t.Parallel()

	svcCtx, _, pub := newTestCtx(t)
	l := NewCreateConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.CreateConversation(&types.CreateConversationReq{Title: "今晚的咨询"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(100), resp.Conversation.UserId)
	assert.Equal(t, "今晚的咨询", resp.Conversation.Title)

	// 断言：发布了 conversation.created 事件
	evts := pub.Events(events.TopicChatEvents)
	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeConversationCreated, evts[0].Type)
	assert.Equal(t, "chat-svc", evts[0].Source)
}

func TestCreateConversationLogic_EmptyTitle_DefaultsToEmpty(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	l := NewCreateConversationLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.CreateConversation(&types.CreateConversationReq{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "", resp.Conversation.Title)
}

func TestCreateConversationLogic_NoUserID_Returns401(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	// 不塞 userID
	l := NewCreateConversationLogic(context.Background(), svcCtx)

	resp, err := l.CreateConversation(&types.CreateConversationReq{Title: "x"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}