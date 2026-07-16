package logic

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/middleware"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestSendMessageLogic_PersistsAndPublishes(t *testing.T) {
	t.Parallel()

	svcCtx, repo, pub := newTestCtx(t)
	// 先创建会话
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100,
		Title:  "test",
	}))

	l := NewSendMessageLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.SendMessage(&types.SendMessageReq{
		Id:      1,
		Role:    "user",
		Content: "我今天心情很低落",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(1), resp.Message.ConversationId)
	assert.Equal(t, "我今天心情很低落", resp.Message.Content)

	// 消息已落库
	msgs, err := repo.ListMessages(context.Background(), 1, 50)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)

	// 会话计数 +1
	conv, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, 1, conv.MessageCount)

	// 发布了 message.created 事件
	evts := pub.Events(events.TopicChatEvents)
	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeMessageCreated, evts[0].Type)
}

func TestSendMessageLogic_ConversationNotFound_Returns404(t *testing.T) {
	t.Parallel()

	svcCtx, _, pub := newTestCtx(t)
	l := NewSendMessageLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.SendMessage(&types.SendMessageReq{Id: 999, Role: "user", Content: "x"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrNotFound)

	// 不应发布事件
	assert.Empty(t, pub.Events(events.TopicChatEvents))
}

func TestSendMessageLogic_EmptyContent_ReturnsValidationError(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	l := NewSendMessageLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.SendMessage(&types.SendMessageReq{Id: 1, Role: "user", Content: ""})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content")
}