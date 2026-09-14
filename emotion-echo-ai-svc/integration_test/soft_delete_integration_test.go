//go:build integration
// +build integration

// Round 1.2 / P2-R2-7: ai-svc EmotionAnalysis 软删除
//
// 背景：i007 已为 emotion_analysis / face_emotion_results / voice_emotion_results /
// fused_emotions 加 deleted_at TIMESTAMPTZ 列；i008（本 PR）补 voice_transcripts。
// 但 GORM model 缺 gorm.DeletedAt 字段，PostgresEmotionRepo.Delete 走物理 DELETE —
// 误删后无法恢复 / 无审计轨迹。
//
// 本测试覆盖：
//   1) EmotionAnalysis model 加 gorm.DeletedAt 字段后，PostgresEmotionRepo.Delete
//      改为软删除（UPDATE ... SET deleted_at = NOW() WHERE id = ?）
//   2) 软删除后 GetByID 返 nil（被 GORM scope 过滤）
//   3) DB 层 SQL 查询（绕过 GORM）仍能看到该行（deleted_at 非 NULL）— 审计可恢复
//
// TDD 步骤：
//   1. RED：本测试运行（PostgresEmotionRepo.Delete 走物理删）— GetByID 失败返错
//   2. GREEN：EmotionAnalysis 加 gorm.DeletedAt + PostgresEmotionRepo.Delete 改软删
//   3. 重跑测试
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
)

// TestEmotionRepo_SoftDelete_GetByIDReturnsNil 软删除后 GetByID 应返 nil（GORM scope 过滤）
func TestEmotionRepo_SoftDelete_GetByIDReturnsNil(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEmotionRepo(db)

	// 1) 插入一条 emotion_analysis
	row := &model.EmotionAnalysis{
		EventID:        "evt-soft-del-1",
		MessageID:      9001,
		UserID:         1,
		ConversationID: 60,
		PrimaryEmotion: "calm",
		SentimentScore: 0.3,
		Model:          "keyword-stub",
	}
	require.NoError(t, repo.Create(ctx, row))
	require.NotZero(t, row.ID, "首次插入应回填 ID")

	// 2) 软删除
	require.NoError(t, repo.Delete(ctx, row.ID))

	// 3) GetByID 期望返 nil（GORM 自动 WHERE deleted_at IS NULL）
	got, err := repo.GetByID(ctx, row.ID)
	require.NoError(t, err, "软删除后 GetByID 不应返 error")
	assert.Nil(t, got, "软删除后 GetByID 应返 nil（被 GORM scope 过滤）")

	// 4) DB 层 SQL 直接查（绕过 GORM scope）应能查到该行（deleted_at 非 NULL）
	var deletedAt *time.Time
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT deleted_at FROM emotion_echo_ai.emotion_analysis WHERE id = ?`, row.ID).
		Scan(&deletedAt).Error)
	require.NotNil(t, deletedAt, "DB 层直接查应能见到该行（deleted_at 非 NULL）— 审计可恢复")
	assert.True(t, deletedAt.After(time.Now().Add(-1*time.Minute)),
		"deleted_at 应在最近 1 分钟内被设置")
}

// TestEmotionRepo_SoftDelete_ListExcludesDeleted 软删除后 ListByConversationID 应排除
func TestEmotionRepo_SoftDelete_ListExcludesDeleted(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEmotionRepo(db)

	convID := int64(70)

	// 1) 插入 2 条 emotion_analysis
	keep := &model.EmotionAnalysis{
		EventID: "evt-keep-1", MessageID: 1, UserID: 1, ConversationID: convID,
		PrimaryEmotion: "calm", Model: "stub",
	}
	del := &model.EmotionAnalysis{
		EventID: "evt-del-1", MessageID: 2, UserID: 1, ConversationID: convID,
		PrimaryEmotion: "sad", Model: "stub",
	}
	require.NoError(t, repo.Create(ctx, keep))
	require.NoError(t, repo.Create(ctx, del))

	// 2) 软删除第 2 条
	require.NoError(t, repo.Delete(ctx, del.ID))

	// 3) ListByConversationID 应只返 1 条
	got, err := repo.ListByConversationID(ctx, convID)
	require.NoError(t, err)
	assert.Len(t, got, 1, "软删除后 ListByConversationID 应只返未删除行")
	if len(got) == 1 {
		assert.Equal(t, "evt-keep-1", got[0].EventID)
	}
}
