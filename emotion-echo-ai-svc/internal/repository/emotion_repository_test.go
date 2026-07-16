package repository

import (
	"context"
	"testing"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmotionRepo_InMemory_CreateAndGet(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID:      100,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
	}))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

func TestEmotionRepo_InMemory_GetByMessageID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 100, UserID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 101, UserID: 1, ConversationID: 50, PrimaryEmotion: "sad",
	}))

	got, err := repo.GetByMessageID(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

func TestEmotionRepo_InMemory_GetByMessageID_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	got, err := repo.GetByMessageID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEmotionRepo_InMemory_ListByConversationID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	// conv=50: 2 条
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 2, ConversationID: 50, PrimaryEmotion: "anxious",
	}))
	// conv=60: 1 条
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 3, ConversationID: 60, PrimaryEmotion: "calm",
	}))

	got, err := repo.ListByConversationID(context.Background(), 50)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	got60, err := repo.ListByConversationID(context.Background(), 60)
	require.NoError(t, err)
	assert.Len(t, got60, 1)

	// 不存在的会话
	got999, err := repo.ListByConversationID(context.Background(), 999)
	require.NoError(t, err)
	assert.Empty(t, got999)
}

func TestEmotionRepo_InMemory_Ping_OK(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Ping(context.Background()))
}