// Package kafka — consumer_test.go
//
// Sibling test for consumer.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 4 part 2 GREEN coverage: handleOne 路由 + remarshal。
// 真实 sarama ConsumerGroup 集成测试需要 Kafka broker，归
// //go:build integration 套件（Round 5 / E2E）。
//
// Coverage matrix:
//
//   - handleOne_MessageCreated_WritesRow
//   - handleOne_ConversationCreated_WritesRow
//   - handleOne_ConversationClosed_WritesRow
//   - handleOne_UnknownType_SkipsNoError
//   - handleOne_InvalidJSON_ReturnsError
//   - remarshal_PreservesFields
package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/events"
	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureEventRepo captures Create() calls for assertion.
type captureEventRepo struct {
	mu    sync.Mutex
	items []*model.UserBehaviorEvent
	err   error
}

func (r *captureEventRepo) Create(_ context.Context, e *model.UserBehaviorEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.items = append(r.items, e)
	return nil
}

func (r *captureEventRepo) GetByID(_ context.Context, _ int64) (*model.UserBehaviorEvent, error) {
	return nil, nil
}

func (r *captureEventRepo) Ping(_ context.Context) error { return nil }

// EventRepo 其他方法 stub）
func (r *captureEventRepo) GetDayNightPattern(_ context.Context, _ int64, _, _ time.Time) (map[int]int64, error) {
	return nil, nil
}

func (r *captureEventRepo) GetInteractionDepth(_ context.Context, _ int64, _, _ time.Time) (*repository.InteractionDepth, error) {
	return nil, nil
}

func (r *captureEventRepo) GetFrequencyTrend(_ context.Context, _ int64, _, _ time.Time) ([]repository.DailyCount, error) {
	return nil, nil
}

// chatEventHandlerForTest exposes the unexported handler for testing.
type chatEventHandlerForTest struct {
	repo repository.EventRepo
}

// 用 reflection / exported wrapper 不可行 — 但 handler 本身是 unexported。
// 我们通过 NewConsumer 构造后用 handleOne 包装一个 exported test helper。
//
// 实际策略 — 把 handleOne 提升为 exported handleEvent（仅测试用）。
// 这里直接通过 consumer.handleOne 的包装调用。

func TestHandleOne_MessageCreated_WritesRow(t *testing.T) {
	t.Parallel()
	repo := &captureEventRepo{}
	h := &chatEventHandler{repo: repo, topic: "chat-events"}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Value: mustJSON(t, events.Event{
			ID:   "evt-1",
			Type: events.EventTypeMessageCreated,
			Time: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
			Data: events.MessageCreatedData{MessageID: 1, ConversationID: 2, UserID: 42},
		}),
	}

	require.NoError(t, h.handleOne(msg))

	require.Len(t, repo.items, 1)
	got := repo.items[0]
	assert.Equal(t, int64(42), got.UserID)
	assert.Equal(t, "message", got.EventType)
	assert.Equal(t, "evt-1", got.Target)
}

func TestHandleOne_ConversationCreated_WritesRow(t *testing.T) {
	t.Parallel()
	repo := &captureEventRepo{}
	h := &chatEventHandler{repo: repo, topic: "chat-events"}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Value: mustJSON(t, events.Event{
			ID:   "evt-2",
			Type: events.EventTypeConversationCreated,
			Time: time.Now(),
			Data: events.ConversationCreatedData{ConversationID: 5, UserID: 99, Title: "hi"},
		}),
	}

	require.NoError(t, h.handleOne(msg))
	require.Len(t, repo.items, 1)
	assert.Equal(t, int64(99), repo.items[0].UserID)
	assert.Equal(t, "conversation", repo.items[0].EventType)
}

func TestHandleOne_ConversationClosed_WritesRow(t *testing.T) {
	t.Parallel()
	repo := &captureEventRepo{}
	h := &chatEventHandler{repo: repo, topic: "chat-events"}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Value: mustJSON(t, events.Event{
			ID:   "evt-3",
			Type: events.EventTypeConversationClosed,
			Time: time.Now(),
			Data: events.ConversationClosedData{ConversationID: 5, UserID: 99},
		}),
	}

	require.NoError(t, h.handleOne(msg))
	require.Len(t, repo.items, 1)
	assert.Equal(t, int64(99), repo.items[0].UserID)
	assert.Equal(t, "conversation", repo.items[0].EventType)
}

func TestHandleOne_UnknownType_SkipsNoError(t *testing.T) {
	t.Parallel()
	repo := &captureEventRepo{}
	h := &chatEventHandler{repo: repo, topic: "chat-events"}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Value: mustJSON(t, events.Event{ID: "evt-99", Type: "unknown.future", Time: time.Now()}),
	}

	require.NoError(t, h.handleOne(msg))
	assert.Empty(t, repo.items)
}

func TestHandleOne_InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := &captureEventRepo{}
	h := &chatEventHandler{repo: repo, topic: "chat-events"}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Value: []byte("{not-json"),
	}
	err := h.handleOne(msg)
	require.Error(t, err)
	assert.Empty(t, repo.items)
}

func TestRemarshal_PreservesFields(t *testing.T) {
	t.Parallel()
	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	src := sample{Name: "alice", Value: 42}

	var dst sample
	require.NoError(t, remarshal(src, &dst))
	assert.Equal(t, src.Name, dst.Name)
	assert.Equal(t, src.Value, dst.Value)
}

// mustJSON marshals v or fatals the test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}