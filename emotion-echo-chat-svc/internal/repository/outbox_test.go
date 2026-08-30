package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInMemoryOutboxRepo_CreateAndListPending 写 3 条 pending → ListPending 拉 3 条
func TestInMemoryOutboxRepo_CreateAndListPending(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
			EventID:   "evt-test-" + string(rune('a'+i)),
			EventType: "conversation.created",
			Topic:     "chat-events",
			Payload:   []byte(`{"id":"evt-test-` + string(rune('a'+i)) + `"}`),
		}))
	}

	pending, err := repo.ListPending(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, pending, 3)

	// 验证默认 status 是 pending
	for _, p := range pending {
		assert.Equal(t, OutboxStatusPending, p.Status)
		assert.Equal(t, 0, p.Attempts)
	}
}

// TestInMemoryOutboxRepo_MarkSent_RemovesFromPending 写 1 条 → MarkSent → ListPending 应为空
func TestInMemoryOutboxRepo_MarkSent_RemovesFromPending(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
		EventID: "evt-mark-sent",
		Topic:   "chat-events",
		Payload: []byte(`{}`),
	}))

	// 标记前应能查到
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	id := pending[0].ID

	// 标记发送
	require.NoError(t, repo.MarkSent(context.Background(), id))

	// 标记后 ListPending 应为空
	pending, _ = repo.ListPending(context.Background(), 10)
	assert.Empty(t, pending, "MarkSent 后该行不应再出现在 pending 列表")

	// 验证 sent_at 已设（Get 按 ID 查）
	got, err := repo.Get(id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, OutboxStatusSent, got.Status)
	assert.NotNil(t, got.SentAt, "sent_at 应被 MarkSent 设置")
}

// TestInMemoryOutboxRepo_MarkFailed_IncrementsAttempts 写 1 条 → MarkFailed → 再读 attempts 应为 1
func TestInMemoryOutboxRepo_MarkFailed_IncrementsAttempts(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
		EventID: "evt-mark-failed",
		Topic:   "chat-events",
		Payload: []byte(`{}`),
	}))
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)

	// MarkFailed 一次
	require.NoError(t, repo.MarkFailed(context.Background(), pending[0].ID, "publish timeout"))

	// 仍然在 pending（status 没变），但 attempts = 1
	pending, _ = repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].Attempts)
	assert.Equal(t, "publish timeout", pending[0].LastError)
	assert.Equal(t, OutboxStatusPending, pending[0].Status, "MarkFailed 不应改 status")
}

// TestInMemoryOutboxRepo_Limit_LimitsResults 写 5 条 → ListPending(2) → 应返 2 条
func TestInMemoryOutboxRepo_Limit_LimitsResults(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
			EventID: "evt-limit-" + string(rune('a'+i)),
			Payload: []byte(`{}`),
		}))
	}
	pending, err := repo.ListPending(context.Background(), 2)
	require.NoError(t, err)
	assert.Len(t, pending, 2)
}

// TestInMemoryOutboxRepo_PayloadIsDeepCopied 验证 CreateInTx 深拷贝 payload（外部修改不影响内部）
func TestInMemoryOutboxRepo_PayloadIsDeepCopied(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	payload := []byte(`{"id":"evt-1"}`)
	require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
		EventID: "evt-1",
		Payload: payload,
	}))

	// 修改外部 slice
	payload[0] = 'X'

	// 内部 payload 应不变
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, `{"id":"evt-1"}`, string(pending[0].Payload), "CreateInTx 应深拷贝 payload")
}

// TestInMemoryOutboxRepo_CreatedAtAutoFilled 验证 CreatedAt 默认填 now
func TestInMemoryOutboxRepo_CreatedAtAutoFilled(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	before := time.Now()
	require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
		EventID: "evt-time",
		Payload: []byte(`{}`),
	}))
	after := time.Now()
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.True(t, !pending[0].CreatedAt.Before(before) && !pending[0].CreatedAt.After(after),
		"CreatedAt 应在 before/after 范围内，实际 %v", pending[0].CreatedAt)
}

// TestInMemoryOutboxRepo_MarkSent_NotFound 验证 MarkSent 不存在的 ID 返 err
func TestInMemoryOutboxRepo_MarkSent_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryOutboxRepo()
	err := repo.MarkSent(context.Background(), 999)
	assert.Error(t, err)
}
