// internal/repository/memory_fact_repository_test.go
//
// CL-08a · A1 · RED：记忆事实仓储（内存实现）的行为契约。
//
// 内存实现与 PG 实现共享同一组语义（幂等 / 双时间轴 / 用户隔离），
// 抽取 worker（A2）只依赖接口 ⇒ 这里的行为就是生产语义的规格。

package repository

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryFactRepo_InMemory_TryMarkExtractRun(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	first, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindMessage, SourceID: 100, UserID: 1, ConversationID: 7,
	})
	require.NoError(t, err)
	assert.True(t, first, "首次登记应为 true（允许继续抽取）")

	second, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindMessage, SourceID: 100, UserID: 1, ConversationID: 7,
	})
	require.NoError(t, err)
	assert.False(t, second, "同一来源第二次登记必须为 false（幂等，避免重复调用 LLM）")
}

func TestMemoryFactRepo_InMemory_TryMarkExtractRun_RetriesFailed(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	_, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindNote, SourceID: 9, UserID: 1,
	})
	require.NoError(t, err)
	require.NoError(t, repo.FinishExtractRun(ctx, model.MemorySourceKindNote, 9, model.MemoryExtractStatusFailed, 0, "boom"))

	again, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindNote, SourceID: 9, UserID: 1,
	})
	require.NoError(t, err)
	assert.True(t, again, "failed 的来源必须允许重试（消费框架重投 / 自述条重试）")
}

// CL-08a 残留修复：进程在抽取途中被杀 / FinishExtractRun(done) 未落库，
// 运行行会停在 pending 且**没有任何调用方会再试**（TryMark 见 pending 就跳过）。
// 语义：pending 仍在租约内 = 有人在跑（跳过，防并发双抽）；超过租约 = 孤儿，允许重投。
func TestMemoryFactRepo_InMemory_TryMarkExtractRun_StalePendingIsRetryable(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()
	key := extractRunKey{kind: model.MemorySourceKindMessage, id: 300}

	_, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindMessage, SourceID: 300, UserID: 1,
	})
	require.NoError(t, err)

	again, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindMessage, SourceID: 300, UserID: 1,
	})
	require.NoError(t, err)
	assert.False(t, again, "租约内的 pending 必须跳过（有 worker 在跑，防并发双抽）")

	// 把运行行做旧，模拟 worker 已死 / 收尾回填失败后无人再试
	repo.mu.Lock()
	repo.runs[key].UpdatedAt = time.Now().UTC().Add(-ExtractRunLease - time.Minute)
	repo.mu.Unlock()

	retried, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{
		SourceKind: model.MemorySourceKindMessage, SourceID: 300, UserID: 1,
	})
	require.NoError(t, err)
	assert.True(t, retried, "超过租约的 pending 必须允许重抽（否则该来源永久不再抽取）")

	run, err := repo.GetExtractRun(ctx, model.MemorySourceKindMessage, 300)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, model.MemoryExtractStatusPending, run.Status)
	assert.False(t, run.UpdatedAt.Before(time.Now().UTC().Add(-time.Minute)), "重抽时必须续租（updated_at 刷新）")
}

func TestMemoryFactRepo_InMemory_FinishExtractRun(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	_, err := repo.TryMarkExtractRun(ctx, &model.MemoryExtractRun{SourceKind: model.MemorySourceKindMessage, SourceID: 100, UserID: 1})
	require.NoError(t, err)
	require.NoError(t, repo.FinishExtractRun(ctx, model.MemorySourceKindMessage, 100, model.MemoryExtractStatusDone, 2, ""))

	run, err := repo.GetExtractRun(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, model.MemoryExtractStatusDone, run.Status)
	assert.Equal(t, 2, run.FactsCount)
}

func TestMemoryFactRepo_InMemory_ApplyFacts_Idempotent(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	plan := PlanFactUpsert(nil, []model.MemoryFact{
		mkFact(0, 1, 100, "用户", "职业", "后端工程师", tPlanBase, ""),
	})
	written, err := repo.ApplyFacts(ctx, plan)
	require.NoError(t, err)
	assert.Equal(t, 1, written)

	written2, err := repo.ApplyFacts(ctx, plan)
	require.NoError(t, err)
	assert.Equal(t, 0, written2, "同一计划的重复应用不得重复入库")

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

func TestMemoryFactRepo_InMemory_SupersedeAndActiveList(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	_, err := repo.ApplyFacts(ctx, PlanFactUpsert(nil, []model.MemoryFact{
		mkFact(0, 1, 100, "用户", "居住地", "北京", tPlanBase, ""),
	}))
	require.NoError(t, err)

	existing, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, existing, 1)

	_, err = repo.ApplyFacts(ctx, PlanFactUpsert(existing, []model.MemoryFact{
		mkFact(0, 1, 200, "用户", "居住地", "上海", tPlanNext, ""),
	}))
	require.NoError(t, err)

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 1, "同一 subject+predicate 只应有一条 active")
	assert.Equal(t, "上海", active[0].Object)

	all, err := repo.ListAllByUser(ctx, 1, 0)
	require.NoError(t, err)
	assert.Len(t, all, 2, "旧事实不得删除（T-15：旧值可追溯）")
	var beijing *model.MemoryFact
	for i := range all {
		if all[i].Object == "北京" {
			beijing = &all[i]
		}
	}
	require.NotNil(t, beijing, "旧事实（北京）必须仍在库里")
	assert.Equal(t, model.MemoryFactStatusSuperseded, beijing.Status)
	require.NotNil(t, beijing.ValidTo)
	assert.True(t, beijing.ValidTo.Equal(tPlanNext), "旧行 valid_to 必须等于新行 valid_from")
}

func TestMemoryFactRepo_InMemory_UserIsolation(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	_, err := repo.ApplyFacts(ctx, PlanFactUpsert(nil, []model.MemoryFact{
		mkFact(0, 1, 100, "用户", "职业", "后端工程师", tPlanBase, ""),
		mkFact(0, 2, 101, "对方", "职业", "设计师", tPlanBase, ""),
	}))
	require.NoError(t, err)

	u1, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, u1, 1, "用户 1 只能看到自己的事实")
	assert.Equal(t, "后端工程师", u1[0].Object)

	u2, err := repo.ListActiveByUser(ctx, 2, 0)
	require.NoError(t, err)
	require.Len(t, u2, 1)
	assert.Equal(t, "设计师", u2[0].Object)
}

func TestMemoryFactRepo_InMemory_ListActiveByConversation(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	f1 := mkFact(0, 1, 100, "用户", "职业", "后端工程师", tPlanBase, "")
	f2 := mkFact(0, 1, 101, "用户", "居住地", "北京", tPlanBase, "")
	f2.ConversationID = 8
	f3 := mkFact(0, 1, 102, "用户", "爱好", "跑步", tPlanBase, "")
	f3.ConversationID = 8

	_, err := repo.ApplyFacts(ctx, PlanFactUpsert(nil, []model.MemoryFact{f1, f2, f3}))
	require.NoError(t, err)

	got, err := repo.ListActiveByConversation(ctx, 8)
	require.NoError(t, err)
	assert.Len(t, got, 2, "会话收尾合并只取本会话抽出的事实")
}

func TestMemoryFactRepo_InMemory_EmptyPlanIsNoop(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	written, err := repo.ApplyFacts(context.Background(), PlanResult{})
	require.NoError(t, err)
	assert.Equal(t, 0, written)
}

func TestMemoryFactRepo_InMemory_ListActiveOrderedByValidFromDesc(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryMemoryFactRepo()
	ctx := context.Background()

	_, err := repo.ApplyFacts(ctx, PlanFactUpsert(nil, []model.MemoryFact{
		mkFact(0, 1, 100, "用户", "居住地", "北京", tPlanBase, ""),
		mkFact(0, 1, 101, "用户", "职业", "后端工程师", tPlanNext, ""),
	}))
	require.NoError(t, err)

	active, err := repo.ListActiveByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.True(t, active[0].ValidFrom.Equal(tPlanNext), "摘要按最近优先排序（valid_from DESC）")

	limited, err := repo.ListActiveByUser(ctx, 1, 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1, "limit 必须生效（摘要预算上游裁剪）")
}
