// internal/repository/memory_fact_planner_test.go
//
// CL-08a · A1 · RED：事实落库计划（纯函数）的行为契约。
//
// 为什么把「失效判定」抽成纯函数：它是本阶段**最容易写错**的一条语义
// （T-3 双时间轴 + T-15 事实更新），但不需要 DB 就能完整覆盖边界。
// 仓储层只负责"照着计划执行"，因此这里的红绿直接决定事实历史的正确性。

package repository

import (
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	tPlanBase = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	tPlanNext = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
)

func mkFact(id, userID, msgID int64, subject, predicate, object string, validFrom time.Time, status string) model.MemoryFact {
	return model.MemoryFact{
		ID:              id,
		UserID:          userID,
		ConversationID:  7,
		SourceMessageID: msgID,
		Subject:         subject,
		Predicate:       predicate,
		Object:          object,
		FactText:        subject + predicate + object,
		ValidFrom:       validFrom,
		Status:          status,
	}
}

func TestPlanFactUpsert_InsertsWhenNoExisting(t *testing.T) {
	t.Parallel()
	in := []model.MemoryFact{mkFact(0, 1, 100, "用户", "职业", "后端工程师", tPlanBase, "")}

	plan := PlanFactUpsert(nil, in)

	assert.Empty(t, plan.Supersede, "没有既有事实 ⇒ 不应失效任何行")
	require.Len(t, plan.Insert, 1)
	assert.Equal(t, model.MemoryFactStatusActive, plan.Insert[0].Status,
		"新事实必须归一化为 active（空 status 不得入库为非法值）")
}

func TestPlanFactUpsert_SupersedesConflictingActiveFact(t *testing.T) {
	t.Parallel()
	existing := []model.MemoryFact{
		mkFact(11, 1, 100, "用户", "居住地", "北京", tPlanBase, model.MemoryFactStatusActive),
	}
	in := []model.MemoryFact{
		mkFact(0, 1, 200, "用户", "居住地", "上海", tPlanNext, ""),
	}

	plan := PlanFactUpsert(existing, in)

	require.Len(t, plan.Supersede, 1, "同 subject+predicate 且 object 不同 ⇒ 旧行必须失效")
	assert.Equal(t, int64(11), plan.Supersede[0].ID)
	assert.True(t, plan.Supersede[0].ValidTo.Equal(tPlanNext),
		"旧行 valid_to 必须等于新行 valid_from（区间首尾相接）")
	require.Len(t, plan.Insert, 1)
	assert.Equal(t, "上海", plan.Insert[0].Object)
}

func TestPlanFactUpsert_SkipsDuplicateOfActiveFact(t *testing.T) {
	t.Parallel()
	existing := []model.MemoryFact{
		mkFact(11, 1, 100, "用户", "职业", "后端工程师", tPlanBase, model.MemoryFactStatusActive),
	}
	// 另一条消息又说了一遍同一事实
	in := []model.MemoryFact{
		mkFact(0, 1, 200, "用户", "职业", "后端工程师", tPlanNext, ""),
	}

	plan := PlanFactUpsert(existing, in)

	assert.Empty(t, plan.Supersede, "值相同 ⇒ 不应失效")
	assert.Empty(t, plan.Insert, "值与既有 active 行相同 ⇒ 不重复插入（避免同一事实多行 active）")
}

func TestPlanFactUpsert_DifferentPredicateNotAffected(t *testing.T) {
	t.Parallel()
	existing := []model.MemoryFact{
		mkFact(11, 1, 100, "用户", "居住地", "北京", tPlanBase, model.MemoryFactStatusActive),
	}
	in := []model.MemoryFact{
		mkFact(0, 1, 200, "用户", "职业", "后端工程师", tPlanNext, ""),
	}

	plan := PlanFactUpsert(existing, in)

	assert.Empty(t, plan.Supersede, "不同 predicate 不得互相失效")
	require.Len(t, plan.Insert, 1)
}

func TestPlanFactUpsert_IgnoresInactiveExisting(t *testing.T) {
	t.Parallel()
	existing := []model.MemoryFact{
		mkFact(11, 1, 100, "用户", "居住地", "北京", tPlanBase, model.MemoryFactStatusSuperseded),
		mkFact(12, 1, 101, "用户", "居住地", "广州", tPlanBase, model.MemoryFactStatusDeleted),
	}
	in := []model.MemoryFact{
		mkFact(0, 1, 200, "用户", "居住地", "上海", tPlanNext, ""),
	}

	plan := PlanFactUpsert(existing, in)

	assert.Empty(t, plan.Supersede, "已 superseded / deleted 的行不得再次失效")
	require.Len(t, plan.Insert, 1)
}

func TestPlanFactUpsert_LastWinsWithinBatch(t *testing.T) {
	t.Parallel()
	in := []model.MemoryFact{
		mkFact(0, 1, 300, "用户", "居住地", "北京", tPlanBase, ""),
		mkFact(0, 1, 300, "用户", "居住地", "上海", tPlanNext, ""),
	}

	plan := PlanFactUpsert(nil, in)

	assert.Empty(t, plan.Supersede, "同一批内不得互相失效（它们都还没入库）")
	require.Len(t, plan.Insert, 1, "同 subject+predicate 只保留一条 active")
	assert.Equal(t, "上海", plan.Insert[0].Object, "同一批内后出现者胜出")
}

func TestPlanFactUpsert_ClampsInvertedInterval(t *testing.T) {
	t.Parallel()
	// 乱序到达：既有行 10-15 生效，新事件却声称 10-01 生效
	existing := []model.MemoryFact{
		mkFact(11, 1, 100, "用户", "居住地", "北京", tPlanNext, model.MemoryFactStatusActive),
	}
	in := []model.MemoryFact{
		mkFact(0, 1, 200, "用户", "居住地", "上海", tPlanBase, ""),
	}

	plan := PlanFactUpsert(existing, in)

	require.Len(t, plan.Supersede, 1)
	assert.False(t, plan.Supersede[0].ValidTo.Before(tPlanNext),
		"valid_to 不得早于旧行 valid_from（否则出现负区间）；应夹到旧行 valid_from")
}

func TestPlanFactUpsert_SkipsEmptyTriple(t *testing.T) {
	t.Parallel()
	in := []model.MemoryFact{
		{UserID: 1, SourceMessageID: 100, Subject: "", Predicate: "职业", Object: "工程师", ValidFrom: tPlanBase},
		{UserID: 1, SourceMessageID: 100, Subject: "用户", Predicate: "", Object: "工程师", ValidFrom: tPlanBase},
		{UserID: 1, SourceMessageID: 100, Subject: "用户", Predicate: "职业", Object: "", ValidFrom: tPlanBase},
	}

	plan := PlanFactUpsert(nil, in)

	assert.Empty(t, plan.Insert, "空三元组不得入库（LLM 解析残缺时宁缺毋滥）")
}
