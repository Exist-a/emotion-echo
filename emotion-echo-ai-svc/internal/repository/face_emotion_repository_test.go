// Stage 34 · PR-1 RED
//
// FaceEmotionRepo 接口契约（行为级，不测 SQL）：
//   - Create(ctx, *FaceEmotionResult) error
//   - GetByUploadID(ctx, uploadID string) (*FaceEmotionResult, error)
//   - GetLatestByMessageID(ctx, messageID int64) (*FaceEmotionResult, error)   -- 多上传取最新
//   - Ping(ctx) error
//
// UploadID 唯一约束幂等：同 UploadID 二次 Create 直接返回 nil，不分配新 id。
// 语义与 Postgres ON CONFLICT (upload_id) DO NOTHING 对齐。
//
// 这些测试**故意**只引用未定义的符号 — 跑必须红。
package repository

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFaceEmotionRepo_InMemory_CreateAndGetByUploadID 基本读写。
func TestFaceEmotionRepo_InMemory_CreateAndGetByUploadID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFaceEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID:       "nonce-001",
		MessageID:      100,
		UserID:         7,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		Confidence:     0.88,
		Model:          "fer:fer",
		CreatedAt:      time.Now(),
	}))

	got, err := repo.GetByUploadID(context.Background(), "nonce-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, 0.88, got.Confidence)
}

// TestFaceEmotionRepo_InMemory_UploadIDDedup 同 UploadID 二次 Create 不分配新 id。
func TestFaceEmotionRepo_InMemory_UploadIDDedup(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFaceEmotionRepo()
	first := &model.FaceEmotionResult{
		UploadID: "nonce-002", UserID: 7, PrimaryEmotion: "happy", Model: "fer:fer",
	}
	require.NoError(t, repo.Create(context.Background(), first))
	originalID := first.ID

	second := &model.FaceEmotionResult{
		UploadID: "nonce-002", UserID: 7, PrimaryEmotion: "sad", Model: "fer:fer",
	}
	require.NoError(t, repo.Create(context.Background(), second))

	// 二次写入后 ID 不变 + 内容仍为 first（与 ON CONFLICT DO NOTHING 语义对齐）
	assert.Equal(t, originalID, second.ID, "duplicate upload_id must not allocate new id")
	got, err := repo.GetByUploadID(context.Background(), "nonce-002")
	require.NoError(t, err)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

// TestFaceEmotionRepo_InMemory_GetLatestByMessageID 多上传取最新（按 created_at desc）。
func TestFaceEmotionRepo_InMemory_GetLatestByMessageID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFaceEmotionRepo()
	now := time.Now()

	require.NoError(t, repo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID: "older", MessageID: 100, UserID: 7, PrimaryEmotion: "neutral",
		CreatedAt: now.Add(-2 * time.Minute),
	}))
	require.NoError(t, repo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID: "newer", MessageID: 100, UserID: 7, PrimaryEmotion: "happy",
		CreatedAt: now,
	}))

	got, err := repo.GetLatestByMessageID(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion, "should return newest")
	assert.Equal(t, "newer", got.UploadID)
}

// TestFaceEmotionRepo_InMemory_GetLatestByMessageID_NotFound 无记录返回 nil。
func TestFaceEmotionRepo_InMemory_GetLatestByMessageID_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFaceEmotionRepo()
	got, err := repo.GetLatestByMessageID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestFaceEmotionRepo_InMemory_Ping 健康检查。
func TestFaceEmotionRepo_InMemory_Ping(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFaceEmotionRepo()
	assert.NoError(t, repo.Ping(context.Background()))
}

// TestFaceEmotionRepo_InterfaceConformance 编译期断言 InMemory 实现满足接口。
func TestFaceEmotionRepo_InterfaceConformance(t *testing.T) {
	t.Parallel()
	var _ FaceEmotionRepo = (*InMemoryFaceEmotionRepo)(nil)
}
