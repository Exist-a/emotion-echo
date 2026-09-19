package repository

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationRepo_CreateAndGet(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	err := repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100,
		Title:  "今天的咨询",
		Status: 1,
	})
	require.NoError(t, err)

	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(100), got.UserID)
	assert.Equal(t, "今天的咨询", got.Title)
}

func TestConversationRepo_Get_NotFound_ReturnsNil(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	got, err := repo.GetConversationByID(context.Background(), 9999)
	require.NoError(t, err)
	assert.Nil(t, got, "missing conversation returns nil, not error")
}

func TestConversationRepo_AppendAndListMessages(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100}))

	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ConversationID: 1,
		UserID:         100,
		Role:           "user",
		Content:        "你好",
	}))
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ConversationID: 1,
		UserID:         100,
		Role:           "assistant",
		Content:        "你好！",
	}))

	msgs, err := repo.ListMessages(context.Background(), 1, 50)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "你好", msgs[0].Content)
	assert.Equal(t, "assistant", msgs[1].Role)
}

func TestConversationRepo_IncrementMessageCount(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100, MessageCount: 0}))

	require.NoError(t, repo.IncrementMessageCount(context.Background(), 1))
	require.NoError(t, repo.IncrementMessageCount(context.Background(), 1))
	require.NoError(t, repo.IncrementMessageCount(context.Background(), 1))

	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3, got.MessageCount)
}

func TestConversationRepo_Ping_OK(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.Ping(context.Background()))
}
func TestConversationRepo_DeleteConversation_RemovesConvAndMessages(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		ID: 1, UserID: 100, Title: "t", Status: 1,
	}))
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ID: 1, ConversationID: 1, UserID: 100, Role: "user", Content: "a",
	}))
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ID: 2, ConversationID: 1, UserID: 100, Role: "user", Content: "b",
	}))

	require.NoError(t, repo.DeleteConversation(context.Background(), 1))

	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got, "会话应已删除")

	msgs, err := repo.ListMessages(context.Background(), 1, 50)
	require.NoError(t, err)
	assert.Empty(t, msgs, "会话消息应级联删除")
}

func TestConversationRepo_DeleteConversation_MissingID_NoError(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.DeleteConversation(context.Background(), 999), "删除不存在的 id 应为 no-op")
}

// =====================================================
// Phase 1a: 软删除语义测试（E2E-08 发现的 bug）
// InMemoryConversationRepo 必须模拟 Postgres 的软删除行为：
// - DeleteConversation 设 deleted_at 而非硬删
// - GetConversationByID / ListConversations / ListMessages 过滤已删除行
// - SetPinned / UpdateTitle 对已删除会话为 no-op
// =====================================================

// TestConversationRepo_SoftDelete_SetsDeletedAt 核心区分测试：
// 软删除后记录必须仍在内存中（DeletedAt 非 nil），而非硬删消失。
// 这是 InMemory 与 Postgres 行为一致性的基石。
func TestConversationRepo_SoftDelete_SetsDeletedAt(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		ID: 1, UserID: 100, Title: "todelete",
	}))
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ID: 10, ConversationID: 1, UserID: 100, Role: "user", Content: "msg",
	}))

	require.NoError(t, repo.DeleteConversation(context.Background(), 1))

	// 直接访问内部状态验证 DeletedAt 被设置（而非从 map 中删除）
	repo.mu.RLock()
	conv := repo.conversations[1]
	msg := repo.messages[10]
	repo.mu.RUnlock()

	require.NotNil(t, conv, "会话记录必须仍在内存中（软删除，非硬删）")
	assert.NotNil(t, conv.DeletedAt, "会话的 DeletedAt 必须被设置")

	require.NotNil(t, msg, "消息记录必须仍在内存中（级联软删除）")
	assert.NotNil(t, msg.DeletedAt, "消息的 DeletedAt 必须被设置")
}

func TestConversationRepo_SoftDelete_GetByID_ReturnsNil(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "todelete",
	}))

	// 删除前可查到
	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)

	// 执行删除
	require.NoError(t, repo.DeleteConversation(context.Background(), 1))

	// 删除后 GetConversationByID 应返回 nil
	got, err = repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got, "软删除后 GetConversationByID 应返回 nil")
}

func TestConversationRepo_SoftDelete_ListConversations_ExcludesDeleted(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100, Title: "keep"}))
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100, Title: "remove"}))

	require.NoError(t, repo.DeleteConversation(context.Background(), 2))

	list, err := repo.ListConversations(context.Background(), 100, 20, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1, "软删除的会话不应出现在列表中")
	assert.Equal(t, "keep", list[0].Title)
}

func TestConversationRepo_SoftDelete_ListMessages_ExcludesDeleted(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{UserID: 100}))
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ID: 1, ConversationID: 1, UserID: 100, Role: "user", Content: "before delete",
	}))

	// 删除会话（级联软删消息）
	require.NoError(t, repo.DeleteConversation(context.Background(), 1))

	msgs, err := repo.ListMessages(context.Background(), 1, 50)
	require.NoError(t, err)
	assert.Empty(t, msgs, "软删除会话的消息不应出现在列表中")
}

func TestConversationRepo_SoftDelete_SetPinned_IsNoOp(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "todelete",
	}))
	require.NoError(t, repo.DeleteConversation(context.Background(), 1))

	// 对已删除会话 SetPinned 应为 no-op，不报错
	require.NoError(t, repo.SetPinned(context.Background(), 1, true))

	// 确认仍然不可见
	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got, "SetPinned 不应复活已删除的会话")
}

func TestConversationRepo_SoftDelete_UpdateTitle_IsNoOp(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "original",
	}))
	require.NoError(t, repo.DeleteConversation(context.Background(), 1))

	// 对已删除会话 UpdateTitle 应为 no-op，不报错
	require.NoError(t, repo.UpdateTitle(context.Background(), 1, "hacked"))

	// 确认仍然不可见
	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got, "UpdateTitle 不应复活已删除的会话")
}
