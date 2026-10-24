//go:build integration
// +build integration

// memory_extract_graph_integration_test.go —— CL-08c Phase 2：
// 抽取流程的 episode/entity 持久化在**真 PG**上的端到端语义。
//
// 与单测（内存替身）的差别：这里验证的是 i012 的唯一索引 + CHECK 约束 +
// GORM 生成的 ON CONFLICT 在完整 handler 流程里真的把幂等挡住了
//（T-4：同一条消息消费三次 ⇒ episode/entity/facts 各 1 行）。

package integration_test

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/logic"
	"emotion-echo-ai-svc/internal/memory"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realMemExtractor 带一条固定事实的假 LLM（集成测试不打网络）。
type realMemExtractor struct{}

func (realMemExtractor) Extract(ctx context.Context, in memory.ExtractInput) ([]model.MemoryFact, error) {
	return []model.MemoryFact{{
		UserID:         in.UserID,
		ConversationID: in.ConversationID,
		SourceKind:     in.SourceKind,
		SourceID:       in.SourceID,
		Subject:        "用户",
		Predicate:      "职业",
		Object:         "后端工程师",
		FactText:       "用户职业后端工程师",
		ValidFrom:      time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
	}}, nil
}

func TestMemoryExtractHandler_GraphPersist_RealPG_Idempotent(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	factRepo := repository.NewPostgresMemoryFactRepo(db)
	graphRepo := repository.NewPostgresMemoryGraphRepo(db)
	h := logic.NewMemoryExtractHandler(factRepo, realMemExtractor{},
		logic.WithGraphRepo(graphRepo))

	evt := &events.Event{
		ID:   "evt-integ",
		Type: events.EventTypeMessageCreated,
		Data: events.MessageCreatedData{
			MessageID: 1000, ConversationID: 7, UserID: 1,
			Role: "user", Content: "我是后端工程师", CreatedAt: 1790000000,
		},
	}

	// T-4：消费三次（at-least-once 重放）
	require.NoError(t, h.Handle(ctx, evt))
	require.NoError(t, h.Handle(ctx, evt))
	require.NoError(t, h.Handle(ctx, evt))

	var n int64
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_ai.memory_episode WHERE user_id = 1`).Scan(&n).Error)
	assert.Equal(t, int64(1), n, "重复消费后 episode 必须只有 1 行")

	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_ai.memory_entity WHERE user_id = 1`).Scan(&n).Error)
	assert.Equal(t, int64(1), n, "重复消费后 entity 必须只有 1 行")

	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_ai.memory_fact WHERE user_id = 1`).Scan(&n).Error)
	assert.Equal(t, int64(1), n, "重复消费后 fact 必须只有 1 行")

	// episode 内容 = 原文（图重建原料）
	ep, err := graphRepo.GetEpisode(ctx, model.MemorySourceKindMessage, 1000)
	require.NoError(t, err)
	require.NotNil(t, ep)
	assert.Equal(t, "我是后端工程师", ep.Content)

	run, err := factRepo.GetExtractRun(ctx, model.MemorySourceKindMessage, 1000)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, model.MemoryExtractStatusDone, run.Status, "第三次消费后运行行应为 done")
}
