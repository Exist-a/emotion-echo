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
	"fmt"
	"sync"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/events"
	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"

	"github.com/IBM/sarama"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================================
// Stage 93 PR-1 RED · analytics-svc consumer sw8 透传测试
// =====================================================
//
// 沿用 ai-svc internal/consumer/consumer_test.go 的 mockTracer / mockSpan 模式
// （PR-OBS-17 + Stage 92 PR-2 同款 mock）。Stage 93 让 analytics-svc 与 ai-svc
// consumer 同模式从 msg.Headers["sw8"] 重建父 trace。
//
// 本段 mock 实现独立于 ai-svc 测试——analytics-svc 不 import ai-svc 内部包，
// mock 镜像一份是为了单仓内测试代码各自可独立编译、独立演进。

// tagKV93 记录 mockSpan93.Tag 调用
type tagKV93 struct{ K, V string }

// mockSpan93 PR-OBS-17 — 满足 grpcinterceptor.Span 接口 + 记录调用
type mockSpan93 struct {
	ended    bool
	endErr   error
	tagCalls []tagKV93
}

func (s *mockSpan93) EndSpan(err error) {
	s.ended = true
	s.endErr = err
}

func (s *mockSpan93) Tag(key, value string) {
	s.tagCalls = append(s.tagCalls, tagKV93{key, value})
}

// SetComponent PR-OBS-19:mock 与接口对齐
func (s *mockSpan93) SetComponent(componentID int32) {}

// SetSpanLayer PR-OBS-19:mock 与接口对齐
func (s *mockSpan93) SetSpanLayer(layer int32) {}

// 编译期断言: mockSpan93 满足 grpcinterceptor.Span
var _ grpcinterceptor.Span = (*mockSpan93)(nil)

// mockTracer93 — 满足 grpcinterceptor.Tracer 接口 + 记录 CreateEntrySpan 调用
//
// 完整实现 StartEntry / CreateLocalSpan / CreateExitSpan / CreateEntrySpan
// (Stage 92 扩展后),其中 CreateEntrySpan 是本测试关注点。
//
// Stage 94 PR-2b §P0-3 扩展：entryFn 可选,若非 nil 则 CreateEntrySpan 走自定义
// 路径(每次返回新 mockSpan)。老测试 entryFn=nil 走原路径(向后兼容)。
type mockTracer93 struct {
	entryOpCalls []string
	entrySw8Seen string
	entrySpan    *mockSpan93
	entryErr     error

	// Stage 94 PR-2b: 自定义 CreateEntrySpan —— 每次返新 mockSpan,让"case 末尾
	// EndSpan"测试可观测多消息 span 独立生命周期
	entryFn func(ctx context.Context, opName string, extractor func(string) (string, error)) (context.Context, grpcinterceptor.Span, error)

	exitOpCalls []string
	exitSpan    *mockSpan93
	exitErr     error
}

func (t *mockTracer93) StartEntry(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span) {
	return ctx, &mockSpan93{}
}

func (t *mockTracer93) CreateLocalSpan(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &mockSpan93{}, nil
}

// CreateEntrySpan Stage 93 — 从 msg.Headers 抽 sw8 → 重建父 trace
//
// Stage 94 PR-2b §P0-3 扩展：entryFn 非 nil 时优先走自定义（每次返回新 mockSpan,
// 让 PR-2b 新加的"span 在 case 末尾 EndSpan"测试可观测多消息 span 独立生命周期）。
func (t *mockTracer93) CreateEntrySpan(
	ctx context.Context, opName string,
	extractor func(string) (string, error),
) (context.Context, grpcinterceptor.Span, error) {
	if t.entryFn != nil {
		return t.entryFn(ctx, opName, extractor)
	}
	t.entryOpCalls = append(t.entryOpCalls, opName)
	if extractor != nil {
		if v, _ := extractor("sw8"); v != "" {
			t.entrySw8Seen = v
		}
	}
	return ctx, t.entrySpan, t.entryErr
}

// CreateExitSpan Stage 92:mock(接口合规所需)
func (t *mockTracer93) CreateExitSpan(
	ctx context.Context, opName, peer string,
	injector func(string, string) error,
) (context.Context, grpcinterceptor.Span, error) {
	t.exitOpCalls = append(t.exitOpCalls, opName)
	return ctx, t.exitSpan, t.exitErr
}

// 编译期断言: mockTracer93 满足 grpcinterceptor.Tracer
var _ grpcinterceptor.Tracer = (*mockTracer93)(nil)

// fakeClaim93 提供可控的 Messages channel(sarama.ConsumerGroupClaim 接口)
type fakeClaim93 struct {
	sarama.ConsumerGroupClaim
	msgs chan *sarama.ConsumerMessage
}

func (f *fakeClaim93) Messages() <-chan *sarama.ConsumerMessage { return f.msgs }

// fakeSession93 模拟 sarama.ConsumerGroupSession,只实现 MarkMessage + Context
//
// Round 5b §B: 加 sync.Mutex 让 MarkMessage 并发安全
type fakeSession93 struct {
	sarama.ConsumerGroupSession
	mu     sync.Mutex
	marked []string
}

func (f *fakeSession93) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, string(msg.Value))
}

func (f *fakeSession93) Context() context.Context { return context.Background() }

// assertHasTag93 断言 tagCalls 含指定 (k,v)
func assertHasTag93(t *testing.T, calls []tagKV93, k, v string) {
	t.Helper()
	for _, kv := range calls {
		if kv.K == k && kv.V == v {
			return
		}
	}
	t.Errorf("expected tag (%q, %q), got calls=%+v", k, v, calls)
}

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

// =====================================================
// Stage 93 PR-1 RED · analytics-svc consumer sw8 透传测试
// =====================================================
//
// 设计参照 ai-svc internal/consumer/consumer_test.go:819
// TestConsumeClaim_RestoresParentTraceFromSw8Header（同语义）：
//   - msg 含 sw8 header → consumer 调 CreateEntrySpan("kafka-consume", ext)
//   - extractor 收到 sw8 值（透传给 go2sky 重建父 SpanContext）
//   - span.EndSpan + 4 个 messaging.* tag
//   - msg 无 sw8 header → CreateEntrySpan 仍调（extractor 返 "" → Valid=false → 新 trace 起点）
//   - Tracer=nil → 跳过 span 创建（向后兼容 Stage 30-A Round 4 原行为）

// driveConsumeClaim93 跑一轮 chatEventHandler.ConsumeClaim 返回。
// 复用 ai-svc consumer_test.go 的 driveConsumeClaim 模式（fakeClaim93/fakeSession93）。
func driveConsumeClaim93(t *testing.T, h *chatEventHandler, msgs ...*sarama.ConsumerMessage) {
	t.Helper()
	claim := &fakeClaim93{msgs: make(chan *sarama.ConsumerMessage, len(msgs))}
	sess := &fakeSession93{}
	for _, m := range msgs {
		claim.msgs <- m
	}
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim timeout")
	}
}

// TestConsumeClaim_RestoresParentTraceFromSw8Header Stage 93 PR-1 RED：
//
// 验证 msg.Headers[sw8] 被抽到并通过 extractor 喂给 CreateEntrySpan；
// span 走 EndSpan + 4 个 messaging.* tag。
func TestConsumeClaim_RestoresParentTraceFromSw8Header(t *testing.T) {
	t.Parallel()
	const fakeSw8 = "1-aabbccdd93eeff0011-2-aabbccdd93-aabbccdd93-aabbccdd93-aabbccdd93"

	span := &mockSpan93{}
	tracer := &mockTracer93{entrySpan: span}

	h := &chatEventHandler{
		repo:       &captureEventRepo{},
		topic:      "chat-events",
		dlq:        NoopDLQPublisher{},
		maxRetries: 3,
		attempts:   make(map[string]int),
	}

	// 通过 WithTracer builder 注入 tracer（GREEN 阶段提供 builder）。
	// 这里直接构造（绕开 builder）保持 RED 测试与 GREEN 实现解耦——只要 GREEN
	// 提供某种方式让 tracer 进入 chatEventHandler 字段，RED 测试都应通过。
	// 沿用 ai-svc 同模式：chatEventHandler.Tracer 字段（builder 模式）
	h.Tracer = tracer

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 5,
		Value:     mustJSON(t, events.Event{ID: "evt-sw8-93", Type: events.EventTypeMessageCreated, Time: time.Now(), Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 7}}),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("sw8"), Value: []byte(fakeSw8)},
			{Key: []byte("content-type"), Value: []byte("application/json")},
		},
		Timestamp: time.Now(),
	}

	driveConsumeClaim93(t, h, msg)

	// 1. 必须调 CreateEntrySpan,operationName=kafka-consume
	if len(tracer.entryOpCalls) != 1 || tracer.entryOpCalls[0] != "kafka-consume" {
		t.Errorf("CreateEntrySpan calls = %v, want [kafka-consume]", tracer.entryOpCalls)
	}
	// 2. extractor 必须抽到 sw8 header value
	if tracer.entrySw8Seen != fakeSw8 {
		t.Errorf("extractor(\"sw8\") = %q, want %q (msg.Headers[sw8] 没被抽到)",
			tracer.entrySw8Seen, fakeSw8)
	}
	// 3. span.EndSpan 必须调（defer）
	if !span.ended {
		t.Error("expected span.EndSpan called (defer 在 ConsumeClaim 内执行)")
	}
	// 4. 4 个 messaging.* tag 精确断言
	assertHasTag93(t, span.tagCalls, "messaging.system", "kafka")
	assertHasTag93(t, span.tagCalls, "messaging.kafka.topic", "chat-events")
	assertHasTag93(t, span.tagCalls, "messaging.kafka.partition", "5")
	assertHasTag93(t, span.tagCalls, "event.type", "message.created")
}

// TestConsumeClaim_NoSw8Header_StillCreatesSpan Stage 93 PR-1 RED 边界用例：
//
// 验证降级语义——msg 无 sw8 header 时 CreateEntrySpan 仍调（extractor 返 "" →
// go2sky Valid=false → 新 trace 起点），不 panic，业务继续。
func TestConsumeClaim_NoSw8Header_StillCreatesSpan(t *testing.T) {
	t.Parallel()
	span := &mockSpan93{}
	tracer := &mockTracer93{entrySpan: span}

	h := &chatEventHandler{
		repo:       &captureEventRepo{},
		topic:      "chat-events",
		dlq:        NoopDLQPublisher{},
		maxRetries: 3,
		attempts:   make(map[string]int),
		Tracer:     tracer,
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     mustJSON(t, events.Event{ID: "evt-no-sw8-93", Type: events.EventTypeMessageCreated, Time: time.Now(), Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 7}}),
		// 无 sw8 header（兼容 Stage 73 之前的旧消息）
		Headers: nil,
	}

	driveConsumeClaim93(t, h, msg)

	// CreateEntrySpan 必调一次（extractor 收 "" → 新 trace 起点）
	if len(tracer.entryOpCalls) != 1 || tracer.entryOpCalls[0] != "kafka-consume" {
		t.Errorf("CreateEntrySpan calls = %v, want [kafka-consume] (无 sw8 仍应调)", tracer.entryOpCalls)
	}
	// extractor 抽 sw8 应返 ""（没命中）
	if tracer.entrySw8Seen != "" {
		t.Errorf("extractor(\"sw8\") = %q, want \"\" (msg 无 sw8 header)", tracer.entrySw8Seen)
	}
	if !span.ended {
		t.Error("expected span.EndSpan called")
	}
}

// TestConsumeClaim_NilTracer_DoesNotCallCreateEntrySpan Stage 93 PR-1 RED 边界用例：
//
// 验证 Tracer=nil 时（向后兼容 Stage 30-A Round 4 原行为）CreateEntrySpan 不调,
// handler 仍正常处理消息,业务不受影响。
func TestConsumeClaim_NilTracer_DoesNotCallCreateEntrySpan(t *testing.T) {
	t.Parallel()
	tracer := &mockTracer93{}
	repo := &captureEventRepo{}

	h := &chatEventHandler{
		repo:       repo,
		topic:      "chat-events",
		dlq:        NoopDLQPublisher{},
		maxRetries: 3,
		attempts:   make(map[string]int),
		Tracer:     nil, // 关键：tracer=nil
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     mustJSON(t, events.Event{ID: "evt-nil-tracer-93", Type: events.EventTypeMessageCreated, Time: time.Now(), Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 7}}),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("sw8"), Value: []byte("1-aabbccdd")},
		},
	}

	driveConsumeClaim93(t, h, msg)

	// Tracer=nil 时 CreateEntrySpan 不应被调
	if len(tracer.entryOpCalls) != 0 {
		t.Errorf("Tracer=nil 时 CreateEntrySpan 不应被调，got calls=%v", tracer.entryOpCalls)
	}
	// 业务不受影响：handler 仍写一条 user_behavior_events 行
	require.Len(t, repo.items, 1, "Tracer=nil 时业务处理必须继续（向后兼容）")
	assert.Equal(t, "evt-nil-tracer-93", repo.items[0].EventID)
}

// TestHandleFailure_ConcurrentAccessIsSafe Round 5b §B GREEN:
//
// observability-edge-gaps §B (consumer.attempts 加锁 P2 0.5h):
// 与 ai-svc TestHandleFailure_ConcurrentAccessIsSafe 同模式:N=50 goroutine
// 并发调 chatEventHandler.handleFailure + ConsumeClaim,断言 attempts map
// 至少 N 个 key 且无 panic。
//
// 测试设计:
//   - 启动 N=50 goroutine 并发调 handleFailure + ConsumeClaim
//   - 断言:attempts map 至少 N 个 key(无锁会偶发 panic + 数据丢失)
//   - 不依赖 -race(Windows 限制,详见 ai-svc 测试注释)
func TestHandleFailure_ConcurrentAccessIsSafe(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	h := &chatEventHandler{
		repo:       &captureEventRepo{},
		topic:      "chat-events",
		dlq:        dlq,
		maxRetries: 100, // 高值,避免任一 goroutine 触发 DLQ 路径
		attempts:   make(map[string]int),
	}

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N * 2)

	// goroutine 1: N 个 handleFailure
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			msg := &sarama.ConsumerMessage{
				Topic: "chat-events",
				Key:   []byte(fmt.Sprintf("evt-an-race-%d", i)),
				Value: mustJSON(t, events.Event{ID: "x", Type: events.EventTypeMessageCreated, Time: time.Now()}),
			}
			sess := &fakeSession93{}
			h.handleFailure(sess, msg, errors.New("forced"))
		}()
	}

	// goroutine 2: N 个 ConsumeClaim
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			msg := &sarama.ConsumerMessage{
				Topic:     "chat-events",
				Key:       []byte(fmt.Sprintf("evt-an-claim-race-%d", i)),
				Value:     mustJSON(t, events.Event{ID: "x", Type: events.EventTypeMessageCreated, Time: time.Now()}),
				Headers:   nil,
				Timestamp: time.Now(),
			}
			claim := &fakeClaim93{msgs: make(chan *sarama.ConsumerMessage, 1)}
			sess := &fakeSession93{}
			claim.msgs <- msg
			close(claim.msgs)
			done := make(chan error, 1)
			go func() { done <- h.ConsumeClaim(sess, claim) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		}()
	}

	wg.Wait()

	if got := len(h.attempts); got < N {
		t.Errorf("attempts map len = %d, want >= %d (handleFailure 写入被并发丢失?)", got, N)
	}
}

// =====================================================
// Stage 94 PR-2b §P0-3 RED · analytics-svc consumer "span 在 case 末尾 EndSpan"
// =====================================================
//
// 与 ai-svc 同名测试同语义：钉死 §P0-3 修复契约 —— "每条消息的 span 必须在 case
// 分支末尾立即 EndSpan"。原 `defer span.EndSpan(nil)` 在 for-loop case 内会延后到
// ConsumeClaim 退出才批量收尾。
//
// 设计：自定义 observeRepo 记录每条 msg 写入时刻所有 span 的 ended 状态。
// 关键差异：handleOne 内部调 repo.Create(),在那一刻观测"上一条 span 是否已 ended"。
// 如果"上一条 span 已 EndSpan",说明 case 末尾立刻收尾（PASS,方案 A）。
// 如果"上一条 span 未 EndSpan",说明 defer 延后（FAIL,旧实现）。
func TestConsumeClaim_SpanEndSpanCalledWithinCaseBody(t *testing.T) {
	t.Parallel()

	// 每次 CreateEntrySpan 返回独立 mockSpan
	spans := make([]*mockSpan93, 3)
	idx := 0
	tracer := &mockTracer93{
		entrySpan: &mockSpan93{}, // 占位（entryFn 非 nil 时 entrySpan 不被读）
		entryFn: func(ctx context.Context, opName string, _ func(string) (string, error)) (context.Context, grpcinterceptor.Span, error) {
			s := &mockSpan93{}
			spans[idx] = s
			idx++
			return ctx, s, nil
		},
	}

	// 自定义 observeRepo:Create 在第 2/3 条 msg 写入时观测 span[0/1] 的 ended 状态
	// 嵌入 captureEventRepo 让其保持 EventRepo 接口合规 + items 累积不变
	repo := &observeRepoP03{
		inner:      &captureEventRepo{},
		spans:      spans,
		prevEnded:  make([]bool, 3),
		invokeIdx:  &idx,
	}

	h := &chatEventHandler{
		repo:       repo,
		topic:      "chat-events",
		dlq:        NoopDLQPublisher{},
		maxRetries: 3,
		attempts:   make(map[string]int),
		Tracer:     tracer,
	}

	// 构造 3 条消息
	mkMsg := func(id string) *sarama.ConsumerMessage {
		return &sarama.ConsumerMessage{
			Topic:     "chat-events",
			Partition: 0,
			Value:     mustJSON(t, events.Event{ID: id, Type: events.EventTypeMessageCreated, Time: time.Now(), Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 7}}),
			Headers:   []*sarama.RecordHeader{{Key: []byte("sw8"), Value: []byte("1-p03b-" + id)}},
		}
	}
	driveConsumeClaim93(t, h,
		mkMsg("evt-1b"), mkMsg("evt-2b"), mkMsg("evt-3b"),
	)

	// 第 2 条消息处理时,span[0] 应已 ended（钉死 case 末尾立刻收尾）
	if !repo.prevEnded[1] {
		t.Error("第 2 条消息处理时 span[0] 必须已 EndSpan（§P0-3 修复契约）；\n" +
			"如 fail 说明 defer 仍留在 case 内（§P0-3 未修）")
	}
	// 第 3 条消息处理时,span[1] 应已 ended
	if !repo.prevEnded[2] {
		t.Error("第 3 条消息处理时 span[1] 必须已 EndSpan（§P0-3 修复契约）")
	}
}

// observeRepoP03 在 Create 时观测"上一条 span 状态"，其余 EventRepo 方法透传。
// invokeIdx 指向 CreateEntrySpan 分配 span 的递增计数器（闭包共享）。
type observeRepoP03 struct {
	inner     *captureEventRepo
	spans     []*mockSpan93
	prevEnded []bool
	invokeIdx *int
}

func (r *observeRepoP03) Create(_ context.Context, e *model.UserBehaviorEvent) error {
	// 此刻 idx 已递增到当前消息;idx-1 是当前 span;idx-2 是上一条
	if *r.invokeIdx >= 2 {
		r.prevEnded[*r.invokeIdx-1] = r.spans[*r.invokeIdx-2].ended
	}
	return r.inner.Create(context.Background(), e)
}

// EventRepo 其他方法 stub
func (r *observeRepoP03) GetByID(_ context.Context, _ int64) (*model.UserBehaviorEvent, error) {
	return nil, nil
}
func (r *observeRepoP03) Ping(_ context.Context) error { return nil }
func (r *observeRepoP03) GetDayNightPattern(_ context.Context, _ int64, _, _ time.Time) (map[int]int64, error) {
	return nil, nil
}
func (r *observeRepoP03) GetInteractionDepth(_ context.Context, _ int64, _, _ time.Time) (*repository.InteractionDepth, error) {
	return nil, nil
}
func (r *observeRepoP03) GetFrequencyTrend(_ context.Context, _ int64, _, _ time.Time) ([]repository.DailyCount, error) {
	return nil, nil
}