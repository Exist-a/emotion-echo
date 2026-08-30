package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRelay_PublishesPendingEntries 写 3 条 pending → FlushOnce → 断言 3 条都 MarkSent
func TestRelay_PublishesPendingEntries(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryOutboxRepo()
	pub := events.NewInMemoryEventPublisher()

	// 写 3 条 pending（payload 是合法 JSON）
	for i, evid := range []string{"evt-relay-1", "evt-relay-2", "evt-relay-3"} {
		require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
			EventID:   evid,
			EventType: "message.created",
			Topic:     "chat-events",
			Payload:   []byte(`{"id":"` + evid + `","type":"message.created","source":"chat-svc","time":"2026-08-30T10:00:00Z","data":{"messageId":` + string(rune('0'+i)) + `,"conversationId":1,"userId":1,"role":"user","content":"hi","createdAt":1}}`),
		}))
	}

	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	require.NoError(t, r.FlushOnce(context.Background()))

	// 断言：3 条都已发布 + MarkSent
	pending, _ := repo.ListPending(context.Background(), 100)
	assert.Empty(t, pending, "所有 pending 应已被 MarkSent")

	gotEvts := pub.Events("chat-events")
	assert.Len(t, gotEvts, 3, "应发布 3 条事件")
}

// TestRelay_MarksFailedOnPublishError publisher 返 err → FlushOnce → 断言 MarkFailed attempts=1
func TestRelay_MarksFailedOnPublishError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryOutboxRepo()
	pub := &failingPublisher{err: errors.New("forced publish err")}

	require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
		EventID:   "evt-fail-1",
		EventType: "message.created",
		Topic:     "chat-events",
		Payload:   []byte(`{"id":"evt-fail-1","type":"message.created","source":"chat-svc"}`),
	}))

	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	require.NoError(t, r.FlushOnce(context.Background()))

	// 断言：仍在 pending（status 不变），但 attempts = 1, last_error = err 信息
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].Attempts)
	assert.Equal(t, "forced publish err", pending[0].LastError)
	assert.Equal(t, repository.OutboxStatusPending, pending[0].Status, "MarkFailed 不应改 status")
}

// TestRelay_BatchSize 写 5 条 → batchSize=2 → FlushOnce 后剩 3 条 pending
func TestRelay_BatchSize(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryOutboxRepo()
	pub := events.NewInMemoryEventPublisher()

	for i := 0; i < 5; i++ {
		evid := "evt-batch-" + string(rune('a'+i))
		require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
			EventID:   evid,
			EventType: "message.created",
			Topic:     "chat-events",
			Payload:   []byte(`{"id":"` + evid + `","type":"message.created","source":"chat-svc"}`),
		}))
	}

	r := NewRelay(repo, pub, 100*time.Millisecond, 2)
	require.NoError(t, r.FlushOnce(context.Background()))

	pending, _ := repo.ListPending(context.Background(), 100)
	assert.Len(t, pending, 3, "batchSize=2 一轮只发 2 条，剩 3 条")
}

// TestRelay_NilPublisher_TreatedAsError publisher=nil 应返 err，entry MarkFailed
func TestRelay_NilPublisher_TreatedAsError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryOutboxRepo()

	require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
		EventID: "evt-nil-pub",
		Topic:   "chat-events",
		Payload: []byte(`{"id":"x","type":"message.created"}`),
	}))

	r := NewRelay(repo, nil, 100*time.Millisecond, 10)
	require.NoError(t, r.FlushOnce(context.Background()))

	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].Attempts, "nil publisher 应 MarkFailed")
	assert.Contains(t, pending[0].LastError, "publisher is nil")
}

// TestRelay_InvalidPayloadJSON_MarksFailed payload 不是合法 JSON → MarkFailed
func TestRelay_InvalidPayloadJSON_MarksFailed(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryOutboxRepo()
	pub := events.NewInMemoryEventPublisher()

	require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
		EventID: "evt-bad-json",
		Topic:   "chat-events",
		Payload: []byte(`{not json}`),
	}))

	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	require.NoError(t, r.FlushOnce(context.Background()))

	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].Attempts)
	assert.Contains(t, pending[0].LastError, "invalid character")
}

// failingPublisher 模拟 publish 失败
type failingPublisher struct {
	err error
}

func (p *failingPublisher) Publish(_ context.Context, _ string, _ *events.Event) error {
	return p.err
}

func (p *failingPublisher) Close() error { return nil }
