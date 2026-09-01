//go:build integration
// +build integration

// Stage 34 · 端到端集成测试
//
// 起真 Postgres（testcontainers）→ apply ai-svc 全套 migrations（含 002/003/004/005）
// → 验 Face / Voice / FusedEmotion repo 在真 Postgres 上的端到端行为。
//
// 覆盖：
//   - UploadID UNIQUE 幂等（Face / Voice）
//   - message_id UNIQUE + Upsert 覆盖（Fused）
//   - GetLatestByMessageID 按 created_at DESC 取最新
//   - VIEW daily_emotion_by_modality_v 三路 UNION ALL 聚合
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

// TestFaceEmotionRepo_Integration_UploadIDDedup
// 验证：PostgresFaceEmotionRepo.Create 同 UploadID 二次调用 → 不会分配新 ID，保留首条
func TestFaceEmotionRepo_Integration_UploadIDDedup(t *testing.T) {
	db, cleanup := pgContainerForEmotion(t, context.Background())
	defer cleanup()

	repo := repository.NewPostgresFaceEmotionRepo(db)

	first := &model.FaceEmotionResult{
		UploadID: "nonce-face-001", UserID: 7,
		MessageID: 100, ConversationID: 50,
		PrimaryEmotion: "happy", Confidence: 0.9, Model: "fer:fer",
	}
	require.NoError(t, repo.Create(context.Background(), first))
	originalID := first.ID

	// 二次 Create 同 UploadID
	second := &model.FaceEmotionResult{
		UploadID: "nonce-face-001", UserID: 7,
		MessageID: 100, ConversationID: 50,
		PrimaryEmotion: "sad", Confidence: 0.5, Model: "fer:fer",
	}
	require.NoError(t, repo.Create(context.Background(), second))

	// 二次写入后 ID 保持不变（Postgres ON CONFLICT DO NOTHING）
	assert.Equal(t, originalID, second.ID, "ON CONFLICT must not allocate new id")

	got, err := repo.GetByUploadID(context.Background(), "nonce-face-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion, "first row must be preserved")
}

// TestFaceEmotionRepo_Integration_GetLatestByMessageID
func TestFaceEmotionRepo_Integration_GetLatestByMessageID(t *testing.T) {
	db, cleanup := pgContainerForEmotion(t, context.Background())
	defer cleanup()

	repo := repository.NewPostgresFaceEmotionRepo(db)
	now := time.Now()

	require.NoError(t, repo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID: "f-older", UserID: 7, MessageID: 200, PrimaryEmotion: "neutral",
		CreatedAt: now.Add(-2 * time.Minute),
	}))
	require.NoError(t, repo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID: "f-newer", UserID: 7, MessageID: 200, PrimaryEmotion: "happy",
		CreatedAt: now,
	}))

	got, err := repo.GetLatestByMessageID(context.Background(), 200)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, "f-newer", got.UploadID)
}

// TestVoiceEmotionRepo_Integration_UploadIDDedup
func TestVoiceEmotionRepo_Integration_UploadIDDedup(t *testing.T) {
	db, cleanup := pgContainerForEmotion(t, context.Background())
	defer cleanup()

	repo := repository.NewPostgresVoiceEmotionRepo(db)

	first := &model.VoiceEmotionResult{
		UploadID: "nonce-voice-001", UserID: 7,
		MessageID: 300, ConversationID: 50,
		Transcript: "你好", PrimaryEmotion: "happy", Confidence: 0.9, Model: "sv:small",
		DurationMs: 3000, Language: "zh",
	}
	require.NoError(t, repo.Create(context.Background(), first))

	// 二次 Create 同 UploadID 应幂等
	require.NoError(t, repo.Create(context.Background(), &model.VoiceEmotionResult{
		UploadID: "nonce-voice-001", UserID: 7,
		PrimaryEmotion: "sad", Confidence: 0.5, Model: "sv:small",
	}))

	got, err := repo.GetByUploadID(context.Background(), "nonce-voice-001")
	require.NoError(t, err)
	assert.Equal(t, "happy", got.PrimaryEmotion, "first row preserved")
	assert.Equal(t, "你好", got.Transcript)
	assert.Equal(t, 3000, got.DurationMs)
	assert.Equal(t, "zh", got.Language)
}

// TestFusedEmotionRepo_Integration_UpsertOverwrites
// 验证：UNIQUE(message_id) + ON CONFLICT DO UPDATE → 二次 Upsert 覆盖（同 ID）
func TestFusedEmotionRepo_Integration_UpsertOverwrites(t *testing.T) {
	db, cleanup := pgContainerForEmotion(t, context.Background())
	defer cleanup()

	repo := repository.NewPostgresFusedEmotionRepo(db)

	// 首次 Upsert（LLM 路径）
	require.NoError(t, repo.Upsert(context.Background(), &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", SentimentScore: -0.55, Confidence: 1.0,
		ModalityContrib:     `{"text":0.5,"voice":0.5}`,
		Reasoning:           "LLM 融合",
		FusionMethod:        "llm",
		AvailableModalities: `["text","voice"]`,
	}))

	// 二次 Upsert（Late 路径，Worker 重试场景）
	require.NoError(t, repo.Upsert(context.Background(), &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "neutral", SentimentScore: 0.0, Confidence: 0.7,
		ModalityContrib:     `{"text":1.0}`,
		Reasoning:           "", // late_fusion 无 LLM 解释
		FusionMethod:        "late_fusion_weighted",
		AvailableModalities: `["text"]`,
	}))

	got, err := repo.GetByMessageID(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "neutral", got.PrimaryEmotion, "Upsert must overwrite")
	assert.Equal(t, "late_fusion_weighted", got.FusionMethod, "fusion_method updated")
	assert.Equal(t, "", got.Reasoning, "reasoning updated (cleared)")
	assert.Equal(t, `["text"]`, got.AvailableModalities)
}

// TestFusedEmotionRepo_Integration_UniqueIndexExists
// 验证：message_id UNIQUE 索引在表中存在（防止误删 migration 004）
//
// 注意：migration 004 用的是 CREATE UNIQUE INDEX（非 ADD CONSTRAINT），
// 所以查 pg_indexes 而不是 pg_constraint。索引名由 migration 显式指定为
// 'uq_fused_emotions_message_id'。
func TestFusedEmotionRepo_Integration_UniqueIndexExists(t *testing.T) {
	db, cleanup := pgContainerForEmotion(t, context.Background())
	defer cleanup()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()

	var count int
	row := sqlDB.QueryRow(`
		SELECT COUNT(*) FROM pg_indexes
		WHERE schemaname = 'emotion_echo_ai'
		  AND tablename = 'fused_emotions'
		  AND indexname = 'uq_fused_emotions_message_id'
	`)
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 1, count, "UNIQUE index must exist from migration 004")
}

// TestDailyEmotionByModalityView_Integration
// 验证：VIEW 三路 UNION ALL 聚合正确
func TestDailyEmotionByModalityView_Integration(t *testing.T) {
	db, cleanup := pgContainerForEmotion(t, context.Background())
	defer cleanup()

	faceRepo := repository.NewPostgresFaceEmotionRepo(db)
	voiceRepo := repository.NewPostgresVoiceEmotionRepo(db)

	// 文本走 emotion_analysis（迁移 002 不写 emotion_analysis，需要直接 SQL）
	require.NoError(t, db.Exec(`
INSERT INTO emotion_echo_ai.emotion_analysis
  (message_id, user_id, conversation_id, primary_emotion, sentiment_score, confidence, model)
VALUES
  (1000, 7, 50, 'sad', -0.5, 0.9, 'text-v1'),
  (1001, 7, 50, 'sad', -0.6, 0.85, 'text-v1'),
  (1002, 7, 50, 'happy', 0.5, 0.9, 'text-v1')
`).Error)

	require.NoError(t, faceRepo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID: "f-1", UserID: 7, MessageID: 1000, PrimaryEmotion: "neutral", Confidence: 0.7, Model: "fer",
	}))
	require.NoError(t, faceRepo.Create(context.Background(), &model.FaceEmotionResult{
		UploadID: "f-2", UserID: 7, MessageID: 1001, PrimaryEmotion: "neutral", Confidence: 0.6, Model: "fer",
	}))

	require.NoError(t, voiceRepo.Create(context.Background(), &model.VoiceEmotionResult{
		UploadID: "v-1", UserID: 7, MessageID: 1000, PrimaryEmotion: "sad", Confidence: 0.8, Model: "sv", DurationMs: 2000, Language: "zh",
	}))

	// 查 VIEW
	var rows []struct {
		Modality     string
		Emotion      string
		Cnt          int
		AvgSentiment *float64
	}
	require.NoError(t, db.Raw(`
		SELECT modality, primary_emotion, cnt, avg_sentiment
		FROM emotion_echo_ai.daily_emotion_by_modality_v
		WHERE user_id = 7
		ORDER BY modality, primary_emotion
	`).Scan(&rows).Error)

	require.GreaterOrEqual(t, len(rows), 3, "VIEW must have at least text/face/voice rows")
	// 至少验证每路都有数据
	modalities := map[string]bool{}
	for _, r := range rows {
		modalities[r.Modality] = true
	}
	assert.True(t, modalities["text"], "text modality present")
	assert.True(t, modalities["face"], "face modality present")
	assert.True(t, modalities["voice"], "voice modality present")
}