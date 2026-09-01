// Stage 34 · PR-3 RED
//
// VoiceEmotionRepo 接口契约：
//   - Create(ctx, *VoiceEmotionResult) error           -- UploadID 幂等
//   - GetByUploadID(ctx, uploadID string) (*VoiceEmotionResult, error)
//   - GetLatestByMessageID(ctx, messageID int64) (*VoiceEmotionResult, error)
//   - Ping(ctx) error
//
// 与 FaceEmotionRepo 同模式（PR-1 已落地的双实现模式）。
package repository

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoiceEmotionRepo_InMemory_CreateAndGetByUploadID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryVoiceEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.VoiceEmotionResult{
		UploadID:       "v-nonce-001",
		MessageID:      200,
		UserID:         7,
		ConversationID: 50,
		Transcript:     "hello",
		PrimaryEmotion: "happy",
		Confidence:     0.9,
		Model:          "sensevoice:sensevoice-small",
		DurationMs:     3000,
		Language:       "zh",
		CreatedAt:      time.Now(),
	}))

	got, err := repo.GetByUploadID(context.Background(), "v-nonce-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, "hello", got.Transcript)
	assert.Equal(t, 3000, got.DurationMs)
}

func TestVoiceEmotionRepo_InMemory_UploadIDDedup(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryVoiceEmotionRepo()
	first := &model.VoiceEmotionResult{
		UploadID: "v-nonce-002", UserID: 7, PrimaryEmotion: "happy", Model: "sensevoice:sensevoice-small",
	}
	require.NoError(t, repo.Create(context.Background(), first))
	originalID := first.ID

	second := &model.VoiceEmotionResult{
		UploadID: "v-nonce-002", UserID: 7, PrimaryEmotion: "sad", Model: "sensevoice:sensevoice-small",
	}
	require.NoError(t, repo.Create(context.Background(), second))

	assert.Equal(t, originalID, second.ID)
	got, err := repo.GetByUploadID(context.Background(), "v-nonce-002")
	require.NoError(t, err)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

func TestVoiceEmotionRepo_InMemory_GetLatestByMessageID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryVoiceEmotionRepo()
	now := time.Now()

	require.NoError(t, repo.Create(context.Background(), &model.VoiceEmotionResult{
		UploadID: "older", MessageID: 200, UserID: 7, PrimaryEmotion: "neutral",
		CreatedAt: now.Add(-3 * time.Minute),
	}))
	require.NoError(t, repo.Create(context.Background(), &model.VoiceEmotionResult{
		UploadID: "newer", MessageID: 200, UserID: 7, PrimaryEmotion: "happy",
		CreatedAt: now,
	}))

	got, err := repo.GetLatestByMessageID(context.Background(), 200)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, "newer", got.UploadID)
}

func TestVoiceEmotionRepo_InMemory_GetLatestByMessageID_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryVoiceEmotionRepo()
	got, err := repo.GetLatestByMessageID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestVoiceEmotionRepo_InMemory_Ping(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryVoiceEmotionRepo()
	assert.NoError(t, repo.Ping(context.Background()))
}

func TestVoiceEmotionRepo_InterfaceConformance(t *testing.T) {
	t.Parallel()
	var _ VoiceEmotionRepo = (*InMemoryVoiceEmotionRepo)(nil)
}
