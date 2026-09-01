package logic

import (
	"context"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stage 36-A2.1 RED：ListConversationsLogic 应能按用户列出最近会话并按 updated_at desc 排序。
//
// 关键约束：
//   - 必须按 user_id 过滤（用户隔离）
//   - 按 updated_at desc 排序（最新活跃在前）
//   - limit + offset 分页（limit 默认 20，offset 默认 0）
//   - 返回 hasMore：取 limit+1 行判断
//
// 失败先写在本文件（RED），等 GREEN 步骤补 ListConversations 方法 + logic。
func TestListConversationsLogic_FiltersByUserID(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)

	uidA, uidB := int64(100), int64(200)
	for i, title := range []string{"A1", "A2"} {
		err := repo.CreateConversation(context.Background(), &model.Conversation{
			UserID:    uidA,
			Title:     title,
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		})
		require.NoError(t, err)
	}
	for i, title := range []string{"B1", "B2"} {
		err := repo.CreateConversation(context.Background(), &model.Conversation{
			UserID:    uidB,
			Title:     title,
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		})
		require.NoError(t, err)
	}

	l := NewListConversationsLogic(ctxWithUserID(context.Background(), uidA), svcCtx)
	resp, err := l.ListConversations(&types.ListConversationsReq{Limit: 20, Offset: 0})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.List, 2, "user A 应只看到自己的 2 个会话")
	for _, item := range resp.List {
		assert.Equal(t, uidA, item.UserId, "不能泄漏其他用户的会话")
	}
	assert.False(t, resp.HasMore, "取 2 条且只有 2 条 → hasMore=false")
}

func TestListConversationsLogic_OrdersByUpdatedDesc(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)
	uid := int64(100)

	// 先插入的 UpdatedAt 早，后插入的晚 → DESC 后插入在前
	c1 := &model.Conversation{UserID: uid, Title: "first", UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	c2 := &model.Conversation{UserID: uid, Title: "second", UpdatedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)}
	err := repo.CreateConversation(context.Background(), c1)
	require.NoError(t, err)
	err = repo.CreateConversation(context.Background(), c2)
	require.NoError(t, err)

	l := NewListConversationsLogic(ctxWithUserID(context.Background(), uid), svcCtx)
	resp, err := l.ListConversations(&types.ListConversationsReq{Limit: 20, Offset: 0})
	require.NoError(t, err)
	require.Len(t, resp.List, 2)

	assert.Equal(t, "second", resp.List[0].Title, "updated_at 更大的应在前")
	assert.Equal(t, "first", resp.List[1].Title)
}

func TestListConversationsLogic_PaginationHasMore(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)
	uid := int64(100)

	for i := 0; i < 5; i++ {
		err := repo.CreateConversation(context.Background(), &model.Conversation{
			UserID:    uid,
			Title:     "c",
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		})
		require.NoError(t, err)
	}

	l := NewListConversationsLogic(ctxWithUserID(context.Background(), uid), svcCtx)

	page1, err := l.ListConversations(&types.ListConversationsReq{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1.List, 2)
	assert.True(t, page1.HasMore, "limit=2 但有 5 条 → hasMore=true")

	page3, err := l.ListConversations(&types.ListConversationsReq{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Len(t, page3.List, 1)
	assert.False(t, page3.HasMore, "offset=4 limit=2 取 5-4=1 条 → hasMore=false")
}

func TestListConversationsLogic_EmptyUser_ReturnsEmptyList(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)

	l := NewListConversationsLogic(ctxWithUserID(context.Background(), int64(999)), svcCtx)
	resp, err := l.ListConversations(&types.ListConversationsReq{Limit: 20, Offset: 0})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.List)
	assert.False(t, resp.HasMore)
}
