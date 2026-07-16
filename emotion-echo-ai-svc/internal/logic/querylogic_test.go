package logic

import (
	"context"
	"testing"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEmotionByMessageLogic_Existing(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID:      100,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.6,
		Confidence:     0.85,
		Model:          "keyword-v1",
	}))

	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewGetEmotionByMessageLogic(context.Background(), svcCtx)

	resp, err := l.GetEmotionByMessage(&types.GetEmotionByMessageReq{MessageId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Emotion)
	assert.Equal(t, "happy", resp.Emotion.PrimaryEmotion)
	assert.Equal(t, int64(100), resp.Emotion.MessageId)
}

func TestGetEmotionByMessageLogic_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewGetEmotionByMessageLogic(context.Background(), svcCtx)

	resp, err := l.GetEmotionByMessage(&types.GetEmotionByMessageReq{MessageId: 999})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListEmotionByConversationLogic_ReturnsAll(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 2, ConversationID: 50, PrimaryEmotion: "anxious",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 3, ConversationID: 60, PrimaryEmotion: "calm", // 不同会话
	}))

	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewListEmotionByConversationLogic(context.Background(), svcCtx)

	resp, err := l.ListEmotionByConversation(&types.ListEmotionByConversationReq{ConversationId: 50})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Emotions, 2)
	assert.Equal(t, "happy", resp.Emotions[0].PrimaryEmotion)
	assert.Equal(t, "anxious", resp.Emotions[1].PrimaryEmotion)
}

func TestListEmotionByConversationLogic_EmptyConv_ReturnsEmptyList(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewListEmotionByConversationLogic(context.Background(), svcCtx)

	resp, err := l.ListEmotionByConversation(&types.ListEmotionByConversationReq{ConversationId: 999})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Emotions, "should return empty slice, not nil")
	assert.Len(t, resp.Emotions, 0)
}