//go:build integration
// +build integration

// Package integration_test — fused_emotion_enum_integration_test.go
//
// AGENTS.md §2.4 §契约 5（schema 与写入端一致性）：
//   「每个 VARCHAR(32) NOT NULL 枚举列，至少 1 个 integration test 断言写入值 ∈ enum 集合」
//
// 本文件覆盖 emotion_echo_ai.fused_emotions.primary_emotion（VARCHAR(32) NOT NULL）。
// 白名单 = internal/fusion/llm_output_validate.go 的 allowedEmotions（9 个标签，
// LLM 融合输出的封闭标签集）。
//
// 断言：① 列宽 = VARCHAR(32) NOT NULL；② 白名单每个情绪经真实 repo Upsert 后原样读回。
//
// 跑：  go test -tags integration -v ./integration_test/ -run TestFusedEmotion
package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
)

// fusedEmotionEnumValues 与 internal/fusion.allowedEmotions 一致（9 标签）。
// integration_test 是独立包，读不到未导出的 allowedEmotions；此字面量即契约，
// 分叉时本测试与 llm_output_validate 的单测会一起红。
var fusedEmotionEnumValues = []string{
	"happy", "sad", "angry", "neutral", "calm", "anxious", "surprised", "disgusted", "fearful",
}

func TestFusedEmotion_Integration_PrimaryEmotionEnumRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, cleanup := newAIDB(t)
	t.Cleanup(cleanup)

	// ① 列宽 = VARCHAR(32) NOT NULL（§契约 5 的"VARCHAR(32)"字面契约）
	var colLen *int
	var nullable string
	require.NoError(t, db.Raw(`SELECT character_maximum_length, is_nullable
		FROM information_schema.columns
		WHERE table_schema='emotion_echo_ai' AND table_name='fused_emotions'
		  AND column_name='primary_emotion'`).Row().Scan(&colLen, &nullable))
	require.NotNil(t, colLen, "fused_emotions.primary_emotion 必须存在")
	assert.Equal(t, 32, *colLen, "primary_emotion 必须是 VARCHAR(32)")
	assert.Equal(t, "NO", nullable, "primary_emotion 必须 NOT NULL")

	repo := repository.NewPostgresFusedEmotionRepo(db)

	// ② 白名单每个情绪经真实写入端往返后原样读回
	for i, want := range fusedEmotionEnumValues {
		require.LessOrEqual(t, len(want), 32, "白名单值 %q 必须塞得进 VARCHAR(32)", want)
		mid := int64(1000 + i)
		f := &model.FusedEmotion{
			MessageID:      mid,
			UserID:         1,
			ConversationID: 1,
			PrimaryEmotion: want,
			SentimentScore: 0,
			Confidence:     0.5,
		}
		require.NoError(t, repo.Upsert(ctx, f), "Upsert primary_emotion=%q 失败", want)

		got, err := repo.GetByMessageID(ctx, mid)
		require.NoError(t, err)
		require.NotNil(t, got, "刚 Upsert 的融合结果必须能读回（primary_emotion=%q）", want)
		assert.Equal(t, want, got.PrimaryEmotion,
			"primary_emotion 往返后必须原样读回（截断/拒绝 = DDL 太窄，§契约 5 类 bug）")
	}
}
