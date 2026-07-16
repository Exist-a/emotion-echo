package repository

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationRepo_CreateAndGet(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	err := repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100,
		Title:  "今天的咨询",
		Status: 1,
	})
	require.NoError(t, err)

	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(100), got.UserID)
	assert.Equal(t, "今天的咨询", got.Title)
}

func TestConversationRepo_Get_NotFound_ReturnsNil(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	got, err := repo.GetConversationByID(context.Background(), 9999)
	require.NoError(t, err)
	assert.Nil(t, got, "missing conversation returns nil, not error")
}

func TestConversationRepo_AppendAndListMessages(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100}))

	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ConversationID: 1,
		UserID:         100,
		Role:           "user",
		Content:        "你好",
	}))
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ConversationID: 1,
		UserID:         100,
		Role:           "assistant",
		Content:        "你好！",
	}))

	msgs, err := repo.ListMessages(context.Background(), 1, 50)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "你好", msgs[0].Content)
	assert.Equal(t, "assistant", msgs[1].Role)
}

func TestConversationRepo_IncrementMessageCount(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100, MessageCount: 0}))

	require.NoError(t, repo.IncrementMessageCount(context.Background(), 1))
	require.NoError(t, repo.IncrementMessageCount(context.Background(), 1))
	require.NoError(t, repo.IncrementMessageCount(context.Background(), 1))

	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3, got.MessageCount)
}

func TestConversationRepo_Ping_OK(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.Ping(context.Background()))
}