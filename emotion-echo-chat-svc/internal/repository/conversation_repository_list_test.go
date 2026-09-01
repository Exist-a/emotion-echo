package repository

import (
	"context"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stage 36-A2.1 RED：ConversationRepo 必须支持按 user_id + limit + offset 列出最近会话，
// 按 updated_at desc 排序，hasMore 由 limit+1 探测。
//
// 这是 repo 层的契约测试；InMemory 实现必须满足，Postgres 实现照抄。
func TestConversationRepo_ListConversations_FiltersByUser(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	uidA, uidB := int64(1), int64(2)

	for i, title := range []string{"A1", "A2"} {
		err := repo.CreateConversation(context.Background(), &model.Conversation{
			UserID:    uidA,
			Title:     title,
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		})
		require.NoError(t, err)
	}
	for _, title := range []string{"B1", "B2"} {
		err := repo.CreateConversation(context.Background(), &model.Conversation{
			UserID:    uidB,
			Title:     title,
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		require.NoError(t, err)
	}

	list, err := repo.ListConversations(context.Background(), uidA, 20, 0)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, c := range list {
		assert.Equal(t, uidA, c.UserID, "用户隔离：不能返回其他用户的会话")
	}
}

func TestConversationRepo_ListConversations_OrdersByUpdatedDesc(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	uid := int64(1)

	err := repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: uid, Title: "older",
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	err = repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: uid, Title: "newer",
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
	})
	require.NoError(t, err)

	list, err := repo.ListConversations(context.Background(), uid, 20, 0)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "newer", list[0].Title, "updated_at 更新的应在最前")
	assert.Equal(t, "older", list[1].Title)
}

func TestConversationRepo_ListConversations_LimitOffset(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	uid := int64(1)

	for i := 0; i < 5; i++ {
		err := repo.CreateConversation(context.Background(), &model.Conversation{
			UserID: uid, Title: "c",
			// 让 i=4 的 updated_at 最大（最近活跃在前）
			UpdatedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
		})
		require.NoError(t, err)
	}

	page1, err := repo.ListConversations(context.Background(), uid, 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page3, err := repo.ListConversations(context.Background(), uid, 2, 4)
	require.NoError(t, err)
	assert.Len(t, page3, 1, "offset=4 limit=2 取第 5 条")
}
