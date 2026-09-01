// Stage 34 · PR-5 RED
//
// FusedEmotionRepo 接口契约：
//   - Upsert(ctx, *FusedEmotion) error                  -- UNIQUE(message_id) 幂等覆盖
//   - GetByMessageID(ctx, messageID int64) (*FusedEmotion, error)
//   - ListPending(ctx, ttlSeconds int) ([]int64, error) -- 找"有 text 但 face/voice 未到"的 messageID
//   - Ping(ctx) error
//
// Upsert 语义：DB 层 ON CONFLICT (message_id) DO UPDATE 覆盖；InMemory 版同语义。
// ListPending 用途：Fusion Worker 每 5s tick 找候选消息。
package repository

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFusedEmotionRepo_InMemory_Upsert_New(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedEmotionRepo()
	f := &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "happy", FusionMethod: "llm",
	}
	require.NoError(t, repo.Upsert(context.Background(), f))
	assert.NotZero(t, f.ID, "Upsert on new message_id must allocate ID")

	got, err := repo.GetByMessageID(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

func TestFusedEmotionRepo_InMemory_Upsert_Overwrite(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedEmotionRepo()
	require.NoError(t, repo.Upsert(context.Background(), &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "happy", FusionMethod: "llm",
	}))
	firstID := repo.(*InMemoryFusedEmotionRepo).byMessageID[100]

	// 二次 Upsert 覆盖（Worker 重试场景）
	require.NoError(t, repo.Upsert(context.Background(), &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", FusionMethod: "late_fusion_weighted",
	}))

	// ID 应保持不变（同一 message_id）
	assert.Equal(t, firstID, repo.(*InMemoryFusedEmotionRepo).byMessageID[100])

	got, err := repo.GetByMessageID(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, "sad", got.PrimaryEmotion, "Upsert must overwrite existing row")
	assert.Equal(t, "late_fusion_weighted", got.FusionMethod)
}

func TestFusedEmotionRepo_InMemory_GetByMessageID_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedEmotionRepo()
	got, err := repo.GetByMessageID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFusedEmotionRepo_InMemory_ListPending_NoFused(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedEmotionRepo()
	// 没 fused 记录时，ListPending 暂返回空（Worker 后续 PR 接入 emotion_analysis 反查）
	ids, err := repo.ListPending(context.Background(), 300)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestFusedEmotionRepo_InMemory_Ping(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedEmotionRepo()
	assert.NoError(t, repo.Ping(context.Background()))
}

func TestFusedEmotionRepo_InterfaceConformance(t *testing.T) {
	t.Parallel()
	var _ FusedEmotionRepo = (*InMemoryFusedEmotionRepo)(nil)
}

// _ = time.Now 防 unused import 警告
var _ = time.Now
