// Package repository — memory_episode_repository_test.go
//
// CL-08c · Phase 1：情节与实体仓储的契约测试（InMemory 替身；PG 侧语义另由
// integration_test/memory_episode_entity_repo_integration_test.go 对真库钉死）。
//
// 钉死的语义（与 Postgres 实现共享）：
//   - 幂等：同一 (source_kind, source_id) 重复 UpsertEpisode 不覆盖、不重复；
//   - 实体唯一：同 (user_id, name) 只一行，重复 upsert 刷新摘要；
//   - 复活：软删的实体被再次抽取 ⇒ 回到 active（用户重新说起新证据应生效）；
//   - 用户隔离：所有列表强制带 user_id，跨用户不可见；
//   - 非法输入：空内容 / 空来源 / 空实体名 ⇒ 报错，不落半行。

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkEpisode(userID, convID, sourceID int64, name, content string, validAt time.Time) *model.MemoryEpisode {
	return &model.MemoryEpisode{
		UserID:         userID,
		ConversationID: convID,
		SourceKind:     model.MemorySourceKindMessage,
		SourceID:       sourceID,
		Name:           name,
		Content:        content,
		ValidAt:        validAt,
	}
}

func TestInMemoryMemoryGraphRepo_UpsertEpisode_RoundTrip(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()
	validAt := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)

	ep := mkEpisode(1, 7, 100, "msg-100", "我最近搬到了上海工作", validAt)
	created, err := repo.UpsertEpisode(ctx, ep)
	require.NoError(t, err)
	assert.True(t, created, "首次写入必须返回 created=true")
	require.NotZero(t, ep.ID, "id 必须由库生成并回填")

	// T-2：写入读回一致（字段逐一比对）
	got, err := repo.GetEpisode(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(1), got.UserID)
	assert.Equal(t, int64(7), got.ConversationID)
	assert.Equal(t, "msg-100", got.Name)
	assert.Equal(t, "我最近搬到了上海工作", got.Content)
	assert.True(t, got.ValidAt.UTC().Equal(validAt), "valid_at 必须原样读回（双时间轴的事件时间）")
	assert.Equal(t, model.MemoryNodeStatusActive, got.Status, "默认 status=active")
	assert.Nil(t, got.DeletedAt)
}

func TestInMemoryMemoryGraphRepo_UpsertEpisode_Idempotent(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()
	validAt := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)

	first, err := repo.UpsertEpisode(ctx, mkEpisode(1, 7, 100, "msg-100", "原文 A", validAt))
	require.NoError(t, err)
	require.True(t, first)

	// 同一来源重复投递（at-least-once）：不得重复、不得覆盖已有内容
	second, err := repo.UpsertEpisode(ctx, mkEpisode(1, 7, 100, "msg-100", "重放时的不同文本", validAt))
	require.NoError(t, err)
	assert.False(t, second, "重复来源必须幂等跳过（created=false）")

	got, err := repo.GetEpisode(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "原文 A", got.Content, "重复 upsert 不得覆盖首次内容")
}

func TestInMemoryMemoryGraphRepo_UpsertEpisode_RejectsInvalidInput(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()
	validAt := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		mut  func(*model.MemoryEpisode)
	}{
		{"empty name", func(e *model.MemoryEpisode) { e.Name = "" }},
		{"empty content", func(e *model.MemoryEpisode) { e.Content = "" }},
		{"empty source kind", func(e *model.MemoryEpisode) { e.SourceKind = "" }},
		{"zero source id", func(e *model.MemoryEpisode) { e.SourceID = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ep := mkEpisode(1, 7, 100, "msg-100", "内容", validAt)
			tc.mut(ep)
			created, err := repo.UpsertEpisode(ctx, ep)
			assert.Error(t, err, "非法输入必须报错，不得落半行")
			assert.False(t, created)
			assert.True(t, errors.Is(err, ErrInvalidMemoryGraph), "必须是 ErrInvalidMemoryGraph")
		})
	}

	// 报错路径不得污染仓储
	got, err := repo.GetEpisode(ctx, model.MemorySourceKindMessage, 100)
	require.NoError(t, err)
	assert.Nil(t, got, "非法输入不得留下任何行")
}

func TestInMemoryMemoryGraphRepo_ListEpisodesByUser_IsolationAndOrder(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()

	t1 := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	_, err := repo.UpsertEpisode(ctx, mkEpisode(1, 7, 100, "msg-100", "更早", t1))
	require.NoError(t, err)
	_, err = repo.UpsertEpisode(ctx, mkEpisode(1, 8, 101, "msg-101", "更晚", t2))
	require.NoError(t, err)
	_, err = repo.UpsertEpisode(ctx, mkEpisode(2, 9, 200, "msg-200", "别人的", t2))
	require.NoError(t, err)

	eps, err := repo.ListEpisodesByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, eps, 2, "用户 1 只能看到自己的两条")
	assert.Equal(t, "更晚", eps[0].Content, "valid_at DESC（最新在前）")
	assert.Equal(t, "更早", eps[1].Content)

	other, err := repo.ListEpisodesByUser(ctx, 2, 0)
	require.NoError(t, err)
	require.Len(t, other, 1, "用户隔离：用户 2 看不到用户 1 的情节")
	assert.Equal(t, "别人的", other[0].Content)
}

func TestInMemoryMemoryGraphRepo_UpsertEntity_CreateRefreshRevive(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()

	// 1) 首次创建
	ent := &model.MemoryEntity{UserID: 1, Name: "母亲", Summary: "用户的母亲"}
	created, err := repo.UpsertEntity(ctx, ent)
	require.NoError(t, err)
	assert.True(t, created)
	require.NotZero(t, ent.ID)

	// 2) 同名重复 upsert ⇒ 刷新摘要、不新增行
	updated := &model.MemoryEntity{UserID: 1, Name: "母亲", Summary: "用户母亲，上周住院"}
	created, err = repo.UpsertEntity(ctx, updated)
	require.NoError(t, err)
	assert.False(t, created, "同名实体必须幂等（created=false）")

	got, err := repo.GetEntityByName(ctx, 1, "母亲")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, inMemoryCountEntities(t, repo, 1), "同 (user, name) 只允许一行")
	assert.Equal(t, "用户母亲，上周住院", got.Summary, "摘要必须被刷新为最新值")

	// 3) 软删后再抽取 ⇒ 复活（新证据应生效）
	inMemorySoftDeleteEntity(t, repo, 1, "母亲")
	got, err = repo.GetEntityByName(ctx, 1, "母亲")
	require.NoError(t, err)
	assert.Nil(t, got, "软删后按名查询不可见")

	revived := &model.MemoryEntity{UserID: 1, Name: "母亲", Summary: "重新提起"}
	created, err = repo.UpsertEntity(ctx, revived)
	require.NoError(t, err)
	assert.True(t, created, "复活视为一次新落行（对调用方语义等价）")

	got, err = repo.GetEntityByName(ctx, 1, "母亲")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, model.MemoryNodeStatusActive, got.Status, "复活后回到 active")
	assert.Nil(t, got.DeletedAt, "复活必须清掉 deleted_at")
	assert.Equal(t, "重新提起", got.Summary)
}

func TestInMemoryMemoryGraphRepo_UpsertEntity_RejectsInvalidInput(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()

	_, err := repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 1, Name: "", Summary: "s"})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidMemoryGraph))

	_, err = repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: 0, Name: "母亲"})
	assert.Error(t, err, "user_id=0 必须拒绝（用户隔离不能是空集）")
}

func TestInMemoryMemoryGraphRepo_ListEntitiesByUser_Isolation(t *testing.T) {
	repo := NewInMemoryMemoryGraphRepo()
	ctx := context.Background()

	for _, u := range []int64{1, 1, 2} {
		_, err := repo.UpsertEntity(ctx, &model.MemoryEntity{UserID: u, Name: "实体", Summary: ""})
		require.NoError(t, err)
	}
	// 同用户同名只一行；跨用户同名是两行
	one, err := repo.ListEntitiesByUser(ctx, 1, 0)
	require.NoError(t, err)
	require.Len(t, one, 1)

	two, err := repo.ListEntitiesByUser(ctx, 2, 0)
	require.NoError(t, err)
	require.Len(t, two, 1)
	assert.NotEqual(t, one[0].ID, two[0].ID, "跨用户同名实体必须是独立行（记忆绝不共享）")
}

// ---- 测试辅助（同包直改内存替身；生产软删入口归 Phase 5 / CL-08d）----

func inMemorySoftDeleteEntity(t *testing.T, repo *InMemoryMemoryGraphRepo, userID int64, name string) {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for _, e := range repo.entities {
		if e.UserID == userID && e.Name == name {
			now := time.Now().UTC()
			e.Status = model.MemoryNodeStatusDeleted
			e.DeletedAt = &now
		}
	}
}

func inMemoryCountEntities(t *testing.T, repo *InMemoryMemoryGraphRepo, userID int64) int {
	t.Helper()
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	n := 0
	for _, e := range repo.entities {
		if e.UserID == userID && e.DeletedAt == nil {
			n++
		}
	}
	return n
}
