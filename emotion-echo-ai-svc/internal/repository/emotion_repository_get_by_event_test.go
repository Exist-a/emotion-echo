package repository

import (
	"context"
	"testing"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stage 36-A3.1 RED：EmotionRepo 必须支持 GetByEventID，用于 UpsertNeutralEmotion
// 实现判断"该 event_id 是否已落库"以返回 was_inserted 标志。
//
// 契约：
//   - 不存在 → 返回 (nil, nil)
//   - 存在 → 返回 (*EmotionAnalysis, nil)
//   - 空 event_id → 视为不存在（与 Create 行为一致：空 event_id 不参与去重）
func TestInMemoryEmotionRepo_GetByEventID_NotFound(t *testing.T) {
	repo := NewInMemoryEmotionRepo()
	got, err := repo.GetByEventID(context.Background(), "evt-missing")
	require.NoError(t, err)
	assert.Nil(t, got, "不存在 → 返回 nil, nil")
}

func TestInMemoryEmotionRepo_GetByEventID_EmptyKey_ReturnsNil(t *testing.T) {
	repo := NewInMemoryEmotionRepo()
	got, err := repo.GetByEventID(context.Background(), "")
	require.NoError(t, err)
	assert.Nil(t, got, "空 event_id → 视为不存在")
}

func TestInMemoryEmotionRepo_GetByEventID_AfterCreate(t *testing.T) {
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 100, UserID: 7, ConversationID: 50,
		EventID:        "evt-uuid-x",
		PrimaryEmotion: "neutral", SentimentScore: 0, Confidence: 0,
		Model: "sync-fallback",
	}))
	got, err := repo.GetByEventID(context.Background(), "evt-uuid-x")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(100), got.MessageID)
	assert.Equal(t, "neutral", got.PrimaryEmotion)
}
