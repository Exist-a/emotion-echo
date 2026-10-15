package repository

import (
	"context"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CL-05 Phase 5 · T-5：会话列表 keyset 游标分页的**复合键**契约。
//
// 稳定序 = `updated_at DESC, id DESC`。若游标只带 updated_at，则"同一时刻更新"的
// 多个会话（同一事务 NOW() 相同 / 种子数据相同）在翻页时会被整批跳过
// （`updated_at < cursor` 对它们恒假）⇒ 漏行。本文件用**同一 updated_at 的多行**
// 构造该边界，这是单键游标一定失败、复合键才通过的判据。
//
// 命名：cursorUpdatedAt 的零值 time.Time{} + cursorID 0 = 首页。

// newConv 造一条会话（id 由 InMemory 仓库自增）。
func newConv(t *testing.T, repo *InMemoryConversationRepo, userID int64, title string, updatedAt time.Time) int64 {
	t.Helper()
	c := &model.Conversation{UserID: userID, Title: title, UpdatedAt: updatedAt}
	require.NoError(t, repo.CreateConversation(context.Background(), c))
	return c.ID
}

func idsOf(cs []model.Conversation) []int64 {
	out := make([]int64, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func TestConversationRepo_ListConversationsPage_FirstPageIsNewestAndHasMore(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	base := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	// 越晚建 updatedAt 越新（id 也随之增大）
	var ids []int64
	for i := 0; i < 5; i++ {
		ids = append(ids, newConv(t, repo, 1, "C", base.Add(time.Duration(i)*time.Minute)))
	}

	page, hasMore, err := repo.ListConversationsPage(context.Background(), 1, 2, time.Time{}, 0)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []int64{ids[4], ids[3]}, idsOf(page), "首页必须取**最新**两条（updated_at DESC）")
	assert.True(t, hasMore)
}

func TestConversationRepo_ListConversationsPage_CompositeCursorSameUpdatedAt(t *testing.T) {
	t.Parallel()

	same := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	repo := NewInMemoryConversationRepo()
	var ids []int64
	for i := 0; i < 3; i++ {
		ids = append(ids, newConv(t, repo, 1, "C", same))
	}

	page1, hasMore, err := repo.ListConversationsPage(context.Background(), 1, 2, time.Time{}, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	// updated_at 相同 ⇒ id DESC 兜底
	assert.Equal(t, []int64{ids[2], ids[1]}, idsOf(page1))
	assert.True(t, hasMore)

	last := page1[len(page1)-1]
	page2, hasMore2, err := repo.ListConversationsPage(context.Background(), 1, 2, last.UpdatedAt, last.ID)
	require.NoError(t, err)
	require.Len(t, page2, 1,
		"同 updated_at 的第 3 条**不得**被跳过 —— 单键游标（只比 updated_at）在这里必然漏行，这正是本钉要抓的缺陷")
	assert.Equal(t, ids[0], page2[0].ID)
	assert.False(t, hasMore2)
}

func TestConversationRepo_ListConversationsPage_WalkAllPagesNoOverlapNoGap(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	base := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	// 刻意混入**同一 updated_at** 的行（模拟同一事务 NOW()）
	stamps := []time.Duration{0, 1 * time.Minute, 1 * time.Minute, 2 * time.Minute, 2 * time.Minute, 2 * time.Minute}
	var want []int64
	for i, d := range stamps {
		_ = i
		want = append(want, newConv(t, repo, 1, "C", base.Add(d)))
	}

	var seen []int64
	var cursorAt time.Time
	var cursorID int64
	for page := 0; page < 10; page++ {
		rows, hasMore, err := repo.ListConversationsPage(context.Background(), 1, 2, cursorAt, cursorID)
		require.NoError(t, err)
		seen = append(seen, idsOf(rows)...)
		if !hasMore {
			break
		}
		require.NotEmpty(t, rows, "hasMore=true 却返回空页 ⇒ 死循环风险")
		last := rows[len(rows)-1]
		cursorAt, cursorID = last.UpdatedAt, last.ID
	}

	assert.Len(t, seen, len(want), "翻完所有页必须拿到全量（不漏）")
	assert.ElementsMatch(t, want, seen, "并集 = 全量（不漏）")
	assert.Equal(t, len(seen), len(uniqueInt64(seen)), "同一行不得跨页重复（不重）")
}

func TestConversationRepo_ListConversationsPage_FiltersUserAndSoftDeleted(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	base := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	mine := newConv(t, repo, 1, "mine", base)
	newConv(t, repo, 2, "other-user", base.Add(time.Hour))
	gone := newConv(t, repo, 1, "deleted", base.Add(2*time.Hour))
	deleted := time.Now()
	repo.conversations[gone].DeletedAt = &deleted

	rows, hasMore, err := repo.ListConversationsPage(context.Background(), 1, 10, time.Time{}, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{mine}, idsOf(rows), "必须按 user 隔离且排除软删除")
	assert.False(t, hasMore)
}

func TestConversationRepo_ListConversationsPage_DefaultLimitIs20(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	base := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		newConv(t, repo, 1, "C", base.Add(time.Duration(i)*time.Second))
	}

	rows, hasMore, err := repo.ListConversationsPage(context.Background(), 1, 0, time.Time{}, 0)
	require.NoError(t, err)
	assert.Len(t, rows, 20, "limit<=0 视为 20（与既有 ListConversations 一致）")
	assert.True(t, hasMore)
}

func uniqueInt64(in []int64) map[int64]struct{} {
	m := make(map[int64]struct{}, len(in))
	for _, v := range in {
		m[v] = struct{}{}
	}
	return m
}
