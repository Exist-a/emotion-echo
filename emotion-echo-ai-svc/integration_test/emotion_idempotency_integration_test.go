//go:build integration
// +build integration

// Stage 30-C A1: ai-svc 消费幂等去重集成测试
//
// 起真 Postgres + apply ai-svc migrations（含 001_add_event_id_to_emotion_analysis.sql）
// + PostgresEmotionRepo.Create 两次同 EventID → 断言表中只有 1 行。
//
// 注意：本次测试需要 ai-svc/migrations/001_*.sql 提供 UNIQUE 约束；migration 文件
// 由本 PR 一并提交。helper 函数 findMigrationsFile 与 analytics-svc 同构（路径
// ../migrations）。
package integration_test

import (
	"context"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
)

// pgContainerForEmotion 保留旧名，委托给共享 fixture（E2E-F-168）。
//
// 旧实现在这里**只手抄了 emotion_analysis 一张表**再跑 ai-svc 迁移，
// 而 `voice_transcripts` / `face_detections` 属于**基础 schema 文件**
// `deploy/db/02-create-tables-in-schemas.sql`、不在 ai-svc 的增量迁移里
// ⇒ 跑完全部迁移后这些表仍不存在，14 个测试全红。
// 现在统一走 testdb_test.go 的 newAIDB：测试库 = 生产库（同源，不会再漂移）。
func pgContainerForEmotion(t *testing.T, _ context.Context) (*gorm.DB, func()) {
	t.Helper()
	return newAIDB(t)
}

// TestEmotionRepo_DuplicateEventID_InsertsOnce PG 端 ON CONFLICT 幂等：同 event_id 两次 Create → 1 行
func TestEmotionRepo_DuplicateEventID_InsertsOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEmotionRepo(db)

	first := &model.EmotionAnalysis{
		EventID:        "evt-int-dup-1",
		MessageID:      1001,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.5,
		Model:          "keyword-stub",
	}
	require.NoError(t, repo.Create(ctx, first))
	require.NotZero(t, first.ID, "首次插入应回填 ID")

	// 第二次同 event_id：不应报错（ON CONFLICT DO NOTHING），也不应产生新行
	second := &model.EmotionAnalysis{
		EventID:        "evt-int-dup-1",
		MessageID:      1001,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.5,
		Model:          "keyword-stub",
	}
	require.NoError(t, repo.Create(ctx, second))

	// 断言：表中只有 1 行
	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.emotion_analysis WHERE event_id = ?`, "evt-int-dup-1").
		Scan(&count).Error)
	assert.Equal(t, int64(1), count, "同 event_id 两次 Create 只应落 1 行")

	// 首次插入的 ID 仍可查到
	got, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "evt-int-dup-1", got.EventID)
}

// TestEmotionRepo_DistinctEventIDs_InsertBoth 反例：不同 event_id 各落一行
func TestEmotionRepo_DistinctEventIDs_InsertBoth(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEmotionRepo(db)
	for _, evid := range []string{"evt-int-a", "evt-int-b"} {
		require.NoError(t, repo.Create(ctx, &model.EmotionAnalysis{
			EventID:   evid,
			MessageID: 1,
			Model:     "keyword-stub",
		}))
	}

	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.emotion_analysis`).
		Scan(&count).Error)
	assert.Equal(t, int64(2), count)
}
