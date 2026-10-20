//go:build integration
// +build integration

// memory_fact_repo_integration_test.go —— CL-08a 残留清销 D.2：
// PG 记忆仓储的**真库**语义（此前只有内存替身被测，PG 实现零 integration test）。
//
// 覆盖 report §2.2 点名的三处未验证语义：
//  1. ApplyFacts 的 supersede：旧行关区间、不删行、只留一条 active；
//  2. TryMarkExtractRun 幂等 + **孤儿 pending 超租约可重试**（D.1 的库侧对偶）；
//  3. 陈旧计划（stale snapshot）下 PG 与内存替身的**语义差异**：
//     PG 由 uq_memory_fact_active_key 拦截 ⇒ 返回错误（可重试）；
//     内存替身静默放过（该差异是已知的，见 stages/cl-08-memory/report.md §2.2），
//     本测试把 PG 侧的行为钉死，避免以后误以为两者完全等价。

package integration_test

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkMemFact(userID, convID, sourceID int64, predicate, object string, validFrom time.Time) model.MemoryFact {
	return model.MemoryFact{
		UserID:         userID,
		ConversationID: convID,
		SourceKind:     model.MemorySourceKindMessage,
		SourceID:       sourceID,
		Subject:        "用户",
		Predicate:      predicate,
		Object:         object,
		FactText:       "用户" + predicate + object,
		ValidFrom:      validFrom,
		Status:         model.MemoryFactStatusActive,
	}
}

func TestPostgresMemoryFactRepo_ApplyFacts_SupersedeKeepsHistory(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryFactRepo(db)

	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	next := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	written, err := repo.ApplyFacts(ctx, repository.PlanFactUpsert(nil, []model.MemoryFact{
		mkMemFact(1, 7, 100, "居住地", "北京", base),
	}))
	require.NoError(t, err)
	require.Equal(t, 1, written)

	existing, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, existing, 1)

	written, err = repo.ApplyFacts(ctx, repository.PlanFactUpsert(existing, []model.MemoryFact{
		mkMemFact(1, 7, 200, "居住地", "上海", next),
	}))
	require.NoError(t, err)
	require.Equal(t, 1, written, "新值插入 1 行")

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1, "同一 (user, subject, predicate) 只允许一条 active（partial unique index）")
	assert.Equal(t, "上海", active[0].Object)

	all, err := repo.ListAllByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, all, 2, "旧事实不得删除（T-15：旧值可追溯）")

	var old *model.MemoryFact
	for i := range all {
		if all[i].Object == "北京" {
			old = &all[i]
		}
	}
	require.NotNil(t, old, "旧行（北京）必须仍在库里")
	assert.Equal(t, model.MemoryFactStatusSuperseded, old.Status)
	require.NotNil(t, old.ValidTo)
	assert.True(t, old.ValidTo.UTC().Equal(next), "旧行 valid_to 必须等于新行 valid_from")
}

func TestPostgresMemoryFactRepo_TryMarkExtractRun_Idempotent(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryFactRepo(db)
	run := func() *model.MemoryExtractRun {
		return &model.MemoryExtractRun{
			SourceKind: model.MemorySourceKindMessage, SourceID: 100, UserID: 1, ConversationID: 7,
		}
	}

	first, err := repo.TryMarkExtractRun(ctx, run())
	require.NoError(t, err)
	assert.True(t, first, "首次登记必须返回 true")

	second, err := repo.TryMarkExtractRun(ctx, run())
	require.NoError(t, err)
	assert.False(t, second, "租约内的 pending 必须跳过（防并发双抽）")

	require.NoError(t, repo.FinishExtractRun(ctx, model.MemorySourceKindMessage, 100, model.MemoryExtractStatusDone, 2, ""))
	third, err := repo.TryMarkExtractRun(ctx, run())
	require.NoError(t, err)
	assert.False(t, third, "done 的来源必须幂等跳过")
}

// D.1 的库侧对偶：孤儿 pending（进程被杀 / 收尾回填失败）超过租约后必须可重试，
// 否则该来源永久不再抽取，且没有任何报错。
func TestPostgresMemoryFactRepo_TryMarkExtractRun_StalePendingIsRetryable(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryFactRepo(db)
	newRun := func() *model.MemoryExtractRun {
		return &model.MemoryExtractRun{
			SourceKind: model.MemorySourceKindNote, SourceID: 42, UserID: 1, ConversationID: 0,
		}
	}

	_, err := repo.TryMarkExtractRun(ctx, newRun())
	require.NoError(t, err)

	// 把运行行做旧到租约之外（模拟 worker 已死）
	require.NoError(t, db.Exec(
		`UPDATE emotion_echo_ai.memory_extract_run SET updated_at = ? WHERE source_kind = ? AND source_id = ?`,
		time.Now().UTC().Add(-repository.ExtractRunLease-time.Minute), model.MemorySourceKindNote, 42,
	).Error)

	retried, err := repo.TryMarkExtractRun(ctx, newRun())
	require.NoError(t, err)
	assert.True(t, retried, "超过租约的 pending 必须允许重抽")

	got, err := repo.GetExtractRun(ctx, model.MemorySourceKindNote, 42)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, model.MemoryExtractStatusPending, got.Status)
	assert.True(t, got.UpdatedAt.After(time.Now().UTC().Add(-time.Minute)), "重抽时必须续租（updated_at 刷新）")
}

// 陈旧计划：两个抽取 worker 各自基于**同一份旧快照**算出计划（都没看到对方的新值）。
// PG 的 uq_memory_fact_active_key 会让第二条插入失败（返回错误 ⇒ 消费框架重投 ⇒ 重算计划后收敛）；
// 内存替身则静默放过（已知差异，见 report §2.2）。
func TestPostgresMemoryFactRepo_ApplyFacts_StalePlanIsRejectedThenConverges(t *testing.T) {
	db, cleanup := newAIDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := repository.NewPostgresMemoryFactRepo(db)

	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	next := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	// 两个 worker 都基于 existing=nil 算出计划
	planA := repository.PlanFactUpsert(nil, []model.MemoryFact{mkMemFact(1, 7, 100, "居住地", "北京", base)})
	planB := repository.PlanFactUpsert(nil, []model.MemoryFact{mkMemFact(1, 7, 200, "居住地", "上海", next)})

	_, err := repo.ApplyFacts(ctx, planA)
	require.NoError(t, err)

	_, err = repo.ApplyFacts(ctx, planB)
	require.Error(t, err, "陈旧计划插入第二条 active 必须被 partial unique index 拦下（不得静默造出两条 active）")

	// 重投：重算计划（这次看得到北京）⇒ 收敛为单条 active
	existing, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, existing, 1, "失败的那次不得留下半状态")

	_, err = repo.ApplyFacts(ctx, repository.PlanFactUpsert(existing, []model.MemoryFact{
		mkMemFact(1, 7, 200, "居住地", "上海", next),
	}))
	require.NoError(t, err)

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "上海", active[0].Object, "重算后新值生效")
}
