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
	"errors"
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
	// ADR-19 Sprint A 全收口（PR-A1.4）: event_type 落库用 ev.Type 原值(带点),
	// 与 chat-svc DevEventPublisher 路径完全一致 → 真正消灭两份映射。
	assert.Equal(t, "message.created", got.EventType,
		"Sprint A 全收口: event_type 必须用 ev.Type 原值(带点),不再 normalize")
	// ADR-19 Sprint A 收口: target 格式统一 chat-svc 风格(msg:N)
	assert.Equal(t, "msg:1", got.Target, "Sprint A: target must be 'msg:N' (chat-svc style), not '1'")
	assert.Equal(t, "conv:2", got.SessionID, "Sprint A: session_id must be 'conv:N' (chat-svc style), not topic name")
	assert.Equal(t, "evt-1", got.EventID, "Stage 30-C A1: EventID 应从 ev.ID 透传")
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
	// ADR-19 Sprint A 全收口: event_type 原值
	assert.Equal(t, "conversation.created", repo.items[0].EventType,
		"Sprint A: event_type must be ev.Type 原值 'conversation.created'")
	// ADR-19 Sprint A: target / session_id 统一 chat-svc 风格
	assert.Equal(t, "conv:5", repo.items[0].Target)
	assert.Equal(t, "conv:5", repo.items[0].SessionID)
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
	// ADR-19 Sprint A 全收口
	assert.Equal(t, "conversation.closed", repo.items[0].EventType,
		"Sprint A: event_type must be ev.Type 原值 'conversation.closed'")
	// ADR-19 Sprint A: target / session_id 统一 chat-svc 风格
	assert.Equal(t, "conv:5", repo.items[0].Target)
	assert.Equal(t, "conv:5", repo.items[0].SessionID)
}

// TestHandleOne_TableDriven_TargetFormatUnified 表驱动覆盖 3 事件类型
//
// Sprint A 全收口 (PR-A1.4): event_type / target / session_id 三字段
// 全部统一 chat-svc 风格 (带点 + msg:N / conv:N + conv:N)。
// 这个表驱动测试钉死 PR-A1.3 v2 + PR-A1.4 的"消灭两份映射"承诺。
func TestHandleOne_TableDriven_TargetFormatUnified(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		eventType     string
		data          events.MessageCreatedData // conv.* 路径只取 ConversationID+UserID
		wantEventType string // PR-A1.4: ev.Type 原值(带点)
		wantTarget    string
		wantSessionID string
	}{
		{
			name:          "message.created",
			eventType:     events.EventTypeMessageCreated,
			data:          events.MessageCreatedData{MessageID: 100, ConversationID: 42, UserID: 7},
			wantEventType: "message.created",
			wantTarget:    "msg:100",
			wantSessionID: "conv:42",
		},
		{
			name:          "conversation.created",
			eventType:     events.EventTypeConversationCreated,
			data:          events.MessageCreatedData{ConversationID: 42, UserID: 7},
			wantEventType: "conversation.created",
			wantTarget:    "conv:42",
			wantSessionID: "conv:42",
		},
		{
			name:          "conversation.closed",
			eventType:     events.EventTypeConversationClosed,
			data:          events.MessageCreatedData{ConversationID: 42, UserID: 7},
			wantEventType: "conversation.closed",
			wantTarget:    "conv:42",
			wantSessionID: "conv:42",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &captureEventRepo{}
			h := &chatEventHandler{repo: repo, topic: "chat-events"}

			var ev events.Event
			ev.ID = "evt-table-" + tc.name
			ev.Type = tc.eventType
			ev.Time = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
			switch tc.eventType {
			case events.EventTypeMessageCreated:
				ev.Data = tc.data
			case events.EventTypeConversationCreated:
				ev.Data = events.ConversationCreatedData{
					ConversationID: tc.data.ConversationID,
					UserID:         tc.data.UserID,
					Title:          "test",
				}
			case events.EventTypeConversationClosed:
				ev.Data = events.ConversationClosedData{
					ConversationID: tc.data.ConversationID,
					UserID:         tc.data.UserID,
				}
			}

			require.NoError(t, h.handleOne(&sarama.ConsumerMessage{
				Topic: "chat-events",
				Value: mustJSON(t, ev),
			}))

			require.Len(t, repo.items, 1)
			got := repo.items[0]
			assert.Equal(t, tc.wantEventType, got.EventType,
				"PR-A1.4: event_type 落库值必须等于 ev.Type 原值(带点)")
			assert.Equal(t, tc.wantTarget, got.Target,
				"target 格式统一 chat-svc 风格 (msg:N / conv:N)")
			assert.Equal(t, tc.wantSessionID, got.SessionID,
				"session_id 格式统一 chat-svc 风格 (conv:N)")
		})
	}
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

// =====================================================
// Stage 30-C A2: DLQ + 重试计数测试（analytics-svc）
// =====================================================

// 嵌入 sarama.ConsumerGroupSession 让测试可以只覆盖 MarkMessage / Context
type stubSession struct {
	sarama.ConsumerGroupSession
	marked *bool
}

func (s stubSession) MarkMessage(*sarama.ConsumerMessage, string) {
	if s.marked != nil {
		*s.marked = true
	}
}

func (s stubSession) Context() context.Context {
	return context.Background()
}

func newStubSession() (sarama.ConsumerGroupSession, *bool) {
	marked := false
	return stubSession{marked: &marked}, &marked
}

// TestHandleOne_DLQ_NoOpWhenSuccess handleOne 成功时直接 Mark，不进 DLQ 路径。
func TestHandleOne_DLQ_NoOpWhenSuccess(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	h := &chatEventHandler{
		repo:       &captureEventRepo{},
		topic:      "chat-events",
		dlq:        dlq,
		maxRetries: 3,
		attempts:   make(map[string]int),
	}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Key:   []byte("evt-success-1"),
		Value: mustJSON(t, events.Event{ID: "evt-success-1", Type: events.EventTypeMessageCreated, Time: time.Now(),
			Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 1}}),
	}
	if err := h.handleOne(msg); err != nil {
		t.Fatalf("handleOne unexpected err: %v", err)
	}
	if got := dlq.Captured(); len(got) != 0 {
		t.Errorf("handleOne 成功不应投 DLQ，got %d entries", len(got))
	}
}

// TestHandleFailure_DLQ_RetriesThenMarks Stage 30-C A2:
// 直接测 handleFailure：第 1/2/3 次不 Mark（attempt <= MaxRetries），
// 第 4 次投 DLQ + Mark + 清 attempts。
func TestHandleFailure_DLQ_RetriesThenMarks(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	h := &chatEventHandler{
		repo:       &captureEventRepo{},
		topic:      "chat-events",
		dlq:        dlq,
		maxRetries: 3,
		attempts:   make(map[string]int),
	}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Key:   []byte("evt-hf-1"),
		Value: []byte(`{"type":"message.created","id":"evt-hf-1","data":{"userId":1}}`),
	}

	for i := 1; i <= 4; i++ {
		sess, marked := newStubSession()
		h.handleFailure(sess, msg, errors.New("forced err"))
		if i <= 3 {
			if *marked {
				t.Errorf("attempt=%d <= MaxRetries=3 时不应 Mark", i)
			}
			if len(dlq.Captured()) != 0 {
				t.Errorf("attempt=%d <= MaxRetries=3 时不应投 DLQ", i)
			}
		} else {
			if !*marked {
				t.Errorf("attempt=%d > MaxRetries=3 时应 Mark", i)
			}
			if len(dlq.Captured()) != 1 {
				t.Errorf("attempt=%d > MaxRetries=3 时应投 DLQ，got %d entries", i, len(dlq.Captured()))
			}
			if h.attempts["evt-hf-1"] != 0 {
				t.Errorf("投 DLQ 后应清 attempts，got %d", h.attempts["evt-hf-1"])
			}
		}
	}

	got := dlq.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(got))
	}
	if got[0].LastError == "" {
		t.Error("DLQ.LastError 应非空")
	}
	if got[0].Attempts != 4 {
		t.Errorf("DLQ.Attempts 应=4，got %d", got[0].Attempts)
	}
	if string(got[0].Value) != string(msg.Value) {
		t.Error("DLQ.Value 应等于原 message.Value")
	}
}

// TestHandleFailure_NoopDLQ_RetriesAndMarks 验证 NoopDLQPublisher 也走 attempt 计数（不退化为"无限重投"）。
func TestHandleFailure_NoopDLQ_RetriesAndMarks(t *testing.T) {
	t.Parallel()
	h := &chatEventHandler{
		repo:       &captureEventRepo{},
		topic:      "chat-events",
		dlq:        NoopDLQPublisher{},
		maxRetries: 1,
		attempts:   make(map[string]int),
	}

	msg := &sarama.ConsumerMessage{
		Topic: "chat-events",
		Key:   []byte("evt-noop-1"),
		Value: []byte(`{"type":"message.created","id":"evt-noop-1","data":{"userId":1}}`),
	}

	sess1, marked1 := newStubSession()
	h.handleFailure(sess1, msg, errors.New("err"))
	if *marked1 {
		t.Error("attempt=1 <= MaxRetries=1 不应 Mark")
	}
	sess2, marked2 := newStubSession()
	h.handleFailure(sess2, msg, errors.New("err"))
	if !*marked2 {
		t.Error("attempt=2 > MaxRetries=1 应 Mark（避免毒消息卡死）")
	}
}

// TestHandleOne_PropagatesEventIDForAllEventTypes Stage 30-C A1 专项断言：
// 三种事件类型都应把 ev.ID 传入 row.EventID。复用现有 captureEventRepo。
func TestHandleOne_PropagatesEventIDForAllEventTypes(t *testing.T) {
	t.Parallel()
	repo := &captureEventRepo{}
	h := &chatEventHandler{repo: repo, topic: "chat-events"}

	now := time.Now().UTC()
	cases := []struct {
		name string
		evt  events.Event
	}{
		{"message.created", events.Event{ID: "evt-mc-1", Type: events.EventTypeMessageCreated, Time: now, Data: events.MessageCreatedData{MessageID: 100, ConversationID: 42, UserID: 1}}},
		{"conversation.created", events.Event{ID: "evt-cc-1", Type: events.EventTypeConversationCreated, Time: now, Data: events.ConversationCreatedData{ConversationID: 42, UserID: 1}}},
		{"conversation.closed", events.Event{ID: "evt-cx-1", Type: events.EventTypeConversationClosed, Time: now, Data: events.ConversationClosedData{ConversationID: 42, UserID: 1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo.items = nil // 清空上一轮
			msg := &sarama.ConsumerMessage{Topic: "chat-events", Value: mustJSON(t, tc.evt)}
			require.NoError(t, h.handleOne(msg))
			require.Len(t, repo.items, 1)
			assert.Equal(t, tc.evt.ID, repo.items[0].EventID,
				"EventID 应等于 ev.ID（Stage 30-C A1 幂等键）")
		})
	}
}