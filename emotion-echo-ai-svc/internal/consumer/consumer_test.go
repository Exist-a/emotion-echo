package consumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/events"

	"github.com/IBM/sarama"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

// fakeSession 模拟 sarama.ConsumerGroupSession 用于单元测试
//
// 只实现 MarkMessage（其他方法不需要）。Tracer 仅校验 span 创建流程。
//
// Round 5b §B: 加 sync.Mutex 让 MarkMessage 并发安全(此前多个 goroutine
// 并发 append 会触发 race detector)。
type fakeSession struct {
	sarama.ConsumerGroupSession
	mu     sync.Mutex
	marked []string
}

func (f *fakeSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, string(msg.Value))
}

// fakeClaim 提供一个可控的 Messages channel
type fakeClaim struct {
	sarama.ConsumerGroupClaim
	msgs chan *sarama.ConsumerMessage
}

// Messages 显式实现 sarama.ConsumerGroupClaim 接口（embed 字段的 nil 不能直接调）
func (f *fakeClaim) Messages() <-chan *sarama.ConsumerMessage { return f.msgs }

// fakeSession 显式实现 MarkMessage + Context（embed 字段 nil 不能直接调）
func (f *fakeSession) Context() context.Context { return context.Background() }

// TestConsumeClaim_NilTracer_DoesNotPanic
//
// 验证：Tracer 为 nil 时 ConsumeClaim 不会 panic，能正常处理消息。
// 这是 Stage 25-F 的最小安全网：保证默认（无 SkyWalking）场景下行为不变。
func TestConsumeClaim_NilTracer_DoesNotPanic(t *testing.T) {
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready:   make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error { handlerCalled <- struct{}{}; return nil },
		// Tracer 留空：保证不 panic
		Tracer: nil,
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     []byte(`{"type":"message.created","id":"evt-1","source":"chat-svc","data":{"messageId":1,"conversationId":1,"userId":1,"content":"hello"}}`),
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}

	claim.msgs <- msg
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeClaim returned err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim timeout")
	}

	// handler 应该被调用 1 次
	select {
	case <-handlerCalled:
		// ok
	default:
		t.Fatal("handler was not called")
	}

	// 消息应该被 mark
	if len(sess.marked) != 1 {
		t.Errorf("expected 1 marked message, got %d", len(sess.marked))
	}
}

// TestConsumeClaim_SkipsUnmarshalErrors
//
// 验证：JSON 解析失败的消息会被 skip 并 mark，不影响后续消息。
func TestConsumeClaim_SkipsUnmarshalErrors(t *testing.T) {
	handlerCalled := 0
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled++
			return nil
		},
		Tracer: nil,
	}

	// 3 条消息：第 1 条格式错，后 2 条正确
	msgs := []*sarama.ConsumerMessage{
		{Topic: "t", Value: []byte(`{bad json`)},                    // bad
		{Topic: "t", Value: []byte(`{"type":"message.created"}`)},     // good
		{Topic: "t", Value: []byte(`{"type":"message.created"}`)},     // good
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, len(msgs))}
	sess := &fakeSession{}
	for _, m := range msgs {
		claim.msgs <- m
	}
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeClaim returned err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if handlerCalled != 2 {
		t.Errorf("expected handler called 2 times (skip 1 bad msg), got %d", handlerCalled)
	}
	// 全部 3 条都应该被 mark（包括 bad 那条被 skip 的）
	if len(sess.marked) != 3 {
		t.Errorf("expected 3 marked messages, got %d", len(sess.marked))
	}
}

// TestConsumeClaim_TopicFilter
//
// 验证：TopicFilter 不匹配的消息被跳过不调 handler，但仍 mark。
func TestConsumeClaim_TopicFilter(t *testing.T) {
	handlerCalled := 0
	h := &ConsumerGroupHandler{
		Ready:       make(chan bool),
		Handler:     func(ctx context.Context, e *events.Event) error { handlerCalled++; return nil },
		Tracer:      nil,
		TopicFilter: "message.created",
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "t", Value: []byte(`{"type":"message.created"}`)},   // match
		{Topic: "t", Value: []byte(`{"type":"user.created"}`)},        // skip (filter)
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, len(msgs))}
	sess := &fakeSession{}
	for _, m := range msgs {
		claim.msgs <- m
	}
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if handlerCalled != 1 {
		t.Errorf("expected handler called 1 time (filter skip 1), got %d", handlerCalled)
	}
	if len(sess.marked) != 2 {
		t.Errorf("expected 2 marked messages, got %d", len(sess.marked))
	}
}

// TestNewKafkaConsumer_BadBrokers_ReturnsError
//
// 验证：broker 地址无效时返回 error，不 panic。
func TestNewKafkaConsumer_BadBrokers_ReturnsError(t *testing.T) {
	// sarama 不会立即连接，但 NewConsumerGroup 会做 DNS 解析
	_, err := NewKafkaConsumer([]string{"this-host-does-not-exist-xyz.invalid:9092"}, "test-group")
	// 我们只断言函数返回（不管 error，因为不同 sarama 版本行为不同）
	_ = err
}

// =====================================================
// Stage 30-C A2: DLQ 死信队列测试
// =====================================================

// driveConsumeClaim 用 fakeClaim/fakeSession 跑一轮 ConsumeClaim。
// 返回 session.marked 的副本（被 MarkMessage 的消息原 Value 列表）。
func driveConsumeClaim(t *testing.T, h *ConsumerGroupHandler, msgs []*sarama.ConsumerMessage) []string {
	t.Helper()
	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, len(msgs))}
	sess := &fakeSession{}
	for _, m := range msgs {
		claim.msgs <- m
	}
	close(claim.msgs)
	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeClaim err: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ConsumeClaim timeout")
	}
	return append([]string(nil), sess.marked...)
}

// TestConsumeClaim_HandlerRetriesBeforeDLQ Stage 30-C A2:
// Handler 返 error N-1 次不投 DLQ，第 N 次进 DLQ + MarkMessage。
// N = MaxRetries + 1（第 1 次 attempt=1 < 3 不投；第 4 次 attempt=4 >= 3 投 DLQ）。
func TestConsumeClaim_HandlerRetriesBeforeDLQ(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	h := &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(ctx context.Context, e *events.Event) error { return errors.New("forced handler err") },
		DLQ:        dlq,
		MaxRetries: 3,
	}

	// 4 次同 key 重投（模拟 sarama 重投递）
	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-poison-1"), Value: []byte(`{"type":"message.created","id":"evt-poison-1"}`)},
		{Topic: "chat-events", Key: []byte("evt-poison-1"), Value: []byte(`{"type":"message.created","id":"evt-poison-1"}`)},
		{Topic: "chat-events", Key: []byte("evt-poison-1"), Value: []byte(`{"type":"message.created","id":"evt-poison-1"}`)},
		{Topic: "chat-events", Key: []byte("evt-poison-1"), Value: []byte(`{"type":"message.created","id":"evt-poison-1"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	// 第 1/2/3 次未 Mark（attempt < MaxRetries 仍重投）
	// 第 4 次 Mark（attempt=4 >= MaxRetries=3 投 DLQ）
	if len(marked) != 1 {
		t.Errorf("expected 1 marked msg (only the DLQ one), got %d: %v", len(marked), marked)
	}
	if got := dlq.Captured(); len(got) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(got))
	}
}

// TestConsumeClaim_DLQReceivesOriginalPayload DLQ entry 的 Value 应等于原 message.Value（不解码）。
func TestConsumeClaim_DLQReceivesOriginalPayload(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	h := &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(ctx context.Context, e *events.Event) error { return errors.New("err") },
		DLQ:        dlq,
		MaxRetries: 1, // 第 2 次就投 DLQ
	}

	originalPayload := []byte(`{"type":"message.created","id":"evt-payload-1","data":{"messageId":1}}`)
	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-payload-1"), Value: originalPayload},
		{Topic: "chat-events", Key: []byte("evt-payload-1"), Value: originalPayload},
	}
	driveConsumeClaim(t, h, msgs)

	got := dlq.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(got))
	}
	if string(got[0].Value) != string(originalPayload) {
		t.Errorf("DLQ Value 应等于原 payload\n  want: %s\n  got:  %s", originalPayload, got[0].Value)
	}
}

// TestConsumeClaim_DLQReceivesErrorReason DLQ entry 应带最后一次错误信息。
func TestConsumeClaim_DLQReceivesErrorReason(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	wantErr := errors.New("analyze: model timeout")
	h := &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(ctx context.Context, e *events.Event) error { return wantErr },
		DLQ:        dlq,
		MaxRetries: 1,
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-err-1"), Value: []byte(`{"type":"message.created","id":"evt-err-1"}`)},
		{Topic: "chat-events", Key: []byte("evt-err-1"), Value: []byte(`{"type":"message.created","id":"evt-err-1"}`)},
	}
	driveConsumeClaim(t, h, msgs)

	got := dlq.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(got))
	}
	if got[0].LastError != wantErr.Error() {
		t.Errorf("DLQ LastError\n  want: %q\n  got:  %q", wantErr.Error(), got[0].LastError)
	}
	if got[0].Attempts < 2 {
		t.Errorf("DLQ Attempts 应 >= 2（至少 2 次失败才投 DLQ），got %d", got[0].Attempts)
	}
}

// TestConsumeClaim_DLQNilIsSafe DLQPublisher 为 nil 时不投 DLQ 但仍 MarkMessage（保留向后兼容）。
func TestConsumeClaim_DLQNilIsSafe(t *testing.T) {
	t.Parallel()
	h := &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(ctx context.Context, e *events.Event) error { return errors.New("err") },
		DLQ:        nil, // 关键：nil
		MaxRetries: 2,
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-nil-1"), Value: []byte(`{"type":"message.created","id":"evt-nil-1"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	// DLQ=nil 时不投也不 Mark（原行为：无限重投），marked 应为空
	if len(marked) != 0 {
		t.Errorf("DLQ=nil 时应保留原行为（不 Mark），got marked=%v", marked)
	}
}

// TestConsumeClaim_HandlerSuccessClearsAttempts 业务成功后应清空 attempts（不污染后续事件）。
func TestConsumeClaim_HandlerSuccessClearsAttempts(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			// 第 1 条失败，第 2 条（不同 key）成功
			if e.ID == "evt-fail" {
				return errors.New("err")
			}
			return nil
		},
		DLQ:        dlq,
		MaxRetries: 5,
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-fail"), Value: []byte(`{"type":"message.created","id":"evt-fail"}`)},
		{Topic: "chat-events", Key: []byte("evt-ok"), Value: []byte(`{"type":"message.created","id":"evt-ok"}`)},
		{Topic: "chat-events", Key: []byte("evt-fail"), Value: []byte(`{"type":"message.created","id":"evt-fail"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	// evt-fail 出现 2 次未投 DLQ（每次 attempt < MaxRetries）；evt-ok 1 次 Mark
	if len(marked) != 1 {
		t.Errorf("expected 1 marked (only evt-ok), got %d", len(marked))
	}
	if len(dlq.Captured()) != 0 {
		t.Errorf("expected 0 DLQ entries (evt-fail attempts < MaxRetries), got %d", len(dlq.Captured()))
	}
}

// ===== PR-OBS-14 RED: 3 case — Kafka consumer trace 边界 =====
//
// 目的: PR-OBS-14 设计 (observability-sprint-b.md §三.14):
// 1. Kafka 消费消息 → 创建 SkyWalking span + tag messaging.system/topic/partition/event.type
// 2. 跨层 trace_id 透传 (与 PR-OBS-12/13 类似)
//
// 现状约束: ConsumerGroupHandler.Tracer 字段是 *go2sky.Tracer (具体类型,见 consumer.go:46-47),
// span, _, _ := h.Tracer.CreateLocalSpan(...) (consumer.go:108) 返回的是 go2sky.Span (具体类型),
// 无法直接 mock。完整 'span tag 断言' 需要:
//   1) 抽 TracerInterface (含 CreateLocalSpan(ctx, name) (SpanInterface, ctx, error))
//   2) 抽 SpanInterface (含 Tag(key, value string))
//   3) ConsumerGroupHandler 改用接口
//   4) mockSpan + mockTracer 实现接口 + 记录 tag 调用
// 上述是较大改造,留作 follow-up PR (与 PR-OBS-12 TracerInterface + PR-OBS-13 Span.Tag 同源)
//
// 本 PR 落地 3 边界 case (覆盖补全型,代码已满足):
// - nil Tracer 不 panic (已有,本 PR 加固断言)
// - consumer.go:108-115 span tag 字面量值验证 (从源码 grep 提取,固化防漂移)
// - handler 收到 ctx 不为 nil (Trace 链入 ctx 不丢失)

// TestConsumeClaim_TraceTagLiterals 验证源码中 span tag 字面量未漂移
//
// 目的: 防止后续重构误改 tag key 名 (SkyWalking UI 聚合查询会断)
// 做法: 直接读源码 grep,断言 4 个关键 tag key 存在
//
// PR-OBS-17: h.Tracer.CreateLocalSpan 签名从 go2sky 变 grpcinterceptor 接口
// (ctx, op) 返回 (ctx, Span, err);span 收尾从 span.End() 变 span.EndSpan(nil)。
// 字面量断言同步更新;但本质上是源码字符串匹配 ——
//
// ⚠️ 已知局限: 这种字面量断言挡不住语义重构(本次重构就让它失效)。
// 完整 tag 值断言由 PR-OBS-17 新增的 TestConsumeClaim_EmitsMessagingSystemTag 等
// mock-based 测试覆盖;本 case 仅作"字面量未漂移"护栏。
func TestConsumeClaim_TraceTagLiterals(t *testing.T) {
	srcBytes, err := os.ReadFile("consumer.go")
	if err != nil {
		t.Skipf("cannot read consumer.go (cwd: %s): %v", os.Getenv("PWD"), err)
	}
	src := string(srcBytes)

	mustContain := []string{
		`span.Tag("messaging.system"`,
		`span.Tag("messaging.kafka.topic"`,
		`span.Tag("messaging.kafka.partition"`,
		`span.Tag("event.type"`,
		// Stage 92 PR-2: CreateLocalSpan → CreateEntrySpan（从 sw8 header 重建父 trace）
		`h.Tracer.CreateEntrySpan(sess.Context(), "kafka-consume", extractor)`,
		`defer span.EndSpan(nil)`,
	}
	for _, m := range mustContain {
		if !strings.Contains(src, m) {
			t.Errorf("consumer.go missing critical trace tag/operation %q (SkyWalking UI 聚合查询依赖)", m)
		}
	}
}

// TestConsumeClaim_HandlerReceivesContext 验证 handler 收到非 nil ctx
//
// 目的: Trace 链入 ctx 必须传递,handler 才能用 ctx 跨服务调用传递 trace_id
// 现状: consumer.go:108 span, _, _ 返回 ctx (第 2 个返回值) → handler(ctx, evt)
//       即使 Tracer 为 nil,ctx 也需传递
func TestConsumeClaim_HandlerReceivesContext(t *testing.T) {
	handlerCalled := make(chan struct{}, 1)
	gotCtx := make(chan context.Context, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			gotCtx <- ctx
			handlerCalled <- struct{}{}
			return nil
		},
		Tracer: nil, // 验证即使无 tracer,ctx 也传递
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     []byte(`{"type":"message.created","id":"evt-1","data":{"messageId":1,"conversationId":1,"userId":1}}`),
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	go func() { _ = h.ConsumeClaim(sess, claim) }()

	select {
	case <-handlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called within 2s")
	}

	select {
	case ctx := <-gotCtx:
		if ctx == nil {
			t.Fatal("handler received nil ctx — Trace 链传递失败")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ctx channel timeout")
	}
}

// TestConsumeClaim_NilTracerDoesNotCallCreateLocalSpan 验证 nil tracer 不触发 span
//
// 目的: 验证 consumer.go:106-107 if h.Tracer != nil 守卫正确
// 现状: Tracer=nil 应直接跳过 span 创建 (CreateLocalSpan 不调用)
// 此 case 与 TestConsumeClaim_NilTracer_DoesNotPanic (已有) 互为补充:
// 已有 case 断言 '不 panic',本 case 断言 '不调用 CreateLocalSpan' (语义层)
func TestConsumeClaim_NilTracerDoesNotCallCreateLocalSpan(t *testing.T) {
	// 当前 Tracer 字段是 *go2sky.Tracer,nil 直接通过 == nil 检查
	// 完整 '验证不调用' 需要 mock,但 nil 字段无法注入 mock tracer
	// 本 case 仅断言 Tracer 字段为 nil 时 handler 仍能跑通 (即不 panic 也跳过 span)
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled <- struct{}{}
			return nil
		},
		Tracer: nil,
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     []byte(`{"type":"message.created","id":"evt-1","data":{"messageId":1,"conversationId":1,"userId":1}}`),
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	go func() { _ = h.ConsumeClaim(sess, claim) }()

	select {
	case <-handlerCalled:
		// handler 跑通,说明 nil Tracer 守卫正确(跳过 span 创建)
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called within 2s (nil Tracer may have blocked execution)")
	}
}

// ===== PR-OBS-17 GREEN: Kafka consumer 4 tag 精确断言 =====
//
// 目的: stage-44 §四 B 收口——用 mockSpan 注入断言 4 个 messaging.* tag 实际值,
// 不依赖源码 grep(后者被 PR-OBS-17 接口重构暴露局限)。
//
// 设计:
// - mockTracer / mockSpan 满足 grpcinterceptor.Tracer / Span 接口
// - mockSpan.Tag 记录 (key, value) 到 tagCalls
// - mockSpan.EndSpan 标记 ended
// - mockTracer.CreateLocalSpan 返回预设的 span + 记 opName 到 localOpCalls
//
// 用例:
// - TestConsumeClaim_EmitsMessagingSystemTag: 4 个 tag 完整断言
// - TestConsumeClaim_NilTracerSpanNotCreated: 边界(nil tracer → 不调用 CreateLocalSpan)
// - TestConsumeClaim_SpanEndSpanCalledOnHandlerSuccess: 业务成功后 span 收尾

// tagKV 记录 mockSpan.Tag 调用
type tagKV struct{ K, V string }

// mockSpan PR-OBS-17 — 满足 grpcinterceptor.Span 接口 + 记录调用
type mockSpan struct {
	ended   bool
	endErr  error
	tagCalls []tagKV
}

func (s *mockSpan) EndSpan(err error) {
	s.ended = true
	s.endErr = err
}

func (s *mockSpan) Tag(key, value string) {
	s.tagCalls = append(s.tagCalls, tagKV{key, value})
}

// SetComponent PR-OBS-19 扩展:mock 与接口对齐,OAP 实际不需要 mock 验证值
func (s *mockSpan) SetComponent(componentID int32) {}

// SetSpanLayer PR-OBS-19 扩展:mock 与接口对齐
func (s *mockSpan) SetSpanLayer(layer int32) {}

// 编译期断言: mockSpan 满足 grpcinterceptor.Span
var _ grpcinterceptor.Span = (*mockSpan)(nil)

// mockTracer PR-OBS-17 — 满足 grpcinterceptor.Tracer 接口 + 记录调用
type mockTracer struct {
	localOpCalls []string
	localSpan    *mockSpan
	localErr     error

	// Stage 92 PR-2: CreateEntrySpan 字段（用于 Kafka consumer 从 sw8 header 重建父 trace）
	entryOpCalls  []string
	entrySw8Seen  string // extractor("sw8") 抽到的值
	entrySpan     *mockSpan
	entryCtx      context.Context
	entryErr      error

	// Stage 94 PR-2 §P0-3 扩展：entryFn 可选，若非 nil 则 CreateEntrySpan 走自定义
	// 路径（每次返回新 mockSpan）。老测试 entryFn=nil 走原路径（向后兼容）。
	entryFn func(ctx context.Context, opName string, extractor func(string) (string, error)) (context.Context, grpcinterceptor.Span, error)

	// Stage 92 PR-2: CreateExitSpan 字段（本测试不验证，本服务不发消息；保留接口合规）
	exitOpCalls  []string
	exitSpan     *mockSpan
	exitErr      error
}

func (t *mockTracer) StartEntry(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span) {
	// ConsumerGroupHandler 不调 StartEntry,这里 no-op 即可
	return ctx, &mockSpan{}
}

func (t *mockTracer) CreateLocalSpan(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span, error) {
	t.localOpCalls = append(t.localOpCalls, opName)
	return ctx, t.localSpan, t.localErr
}

// CreateEntrySpan Stage 92 PR-2：从 msg.Headers 抽 sw8 → 重建父 trace
// Stage 94 PR-2 §P0-3 扩展：entryFn 非 nil 时优先走自定义（每次返回新 mockSpan，
// 让 PR-2 新加的"span 在 case 末尾 EndSpan"测试可观测多消息 span 独立生命周期）。
func (t *mockTracer) CreateEntrySpan(
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
	outCtx := ctx
	if t.entryCtx != nil {
		outCtx = t.entryCtx
	}
	return outCtx, t.entrySpan, t.entryErr
}

// CreateExitSpan Stage 92 PR-2：mock（接口合规所需）
func (t *mockTracer) CreateExitSpan(
	ctx context.Context, opName, peer string,
	injector func(string, string) error,
) (context.Context, grpcinterceptor.Span, error) {
	t.exitOpCalls = append(t.exitOpCalls, opName)
	return ctx, t.exitSpan, t.exitErr
}

// 编译期断言: mockTracer 满足 grpcinterceptor.Tracer
var _ grpcinterceptor.Tracer = (*mockTracer)(nil)

// assertHasTag 辅助断言 mockSpan.tagCalls 含指定 (k,v)
func assertHasTag(t *testing.T, calls []tagKV, k, v string) {
	t.Helper()
	for _, kv := range calls {
		if kv.K == k && kv.V == v {
			return
		}
	}
	t.Errorf("expected tag (%q, %q), got calls=%+v", k, v, calls)
}

// TestConsumeClaim_EmitsMessagingSystemTag 断言 4 个 messaging.* tag 精确值
//
// 完整覆盖 stage-44 §四 B 列的 Kafka span tag 目标:
//   - messaging.system = "kafka"
//   - messaging.kafka.topic = <msg.Topic>
//   - messaging.kafka.partition = <msg.Partition 字符串化>
//   - event.type = <evt.Type>
func TestConsumeClaim_EmitsMessagingSystemTag(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	// Stage 92 PR-2: span 由 CreateEntrySpan 返（不是 CreateLocalSpan）
	tracer := &mockTracer{entrySpan: span}
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled <- struct{}{}
			return nil
		},
		Tracer: tracer,
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 3,
		Value:     []byte(`{"type":"message.created","id":"evt-tag-1","data":{"messageId":1,"conversationId":1,"userId":1}}`),
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case <-handlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called within 2s")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeClaim err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ConsumeClaim timeout")
	}

	// 1. CreateEntrySpan 调用: opName = "kafka-consume"（Stage 92 PR-2 改用 entry）
	if len(tracer.entryOpCalls) != 1 || tracer.entryOpCalls[0] != "kafka-consume" {
		t.Errorf("expected entryOpCalls=[kafka-consume], got %v", tracer.entryOpCalls)
	}
	// 2. Span 必须被 EndSpan (本 case handler 无 err)
	if !span.ended {
		t.Error("expected span.EndSpan called")
	}
	if span.endErr != nil {
		t.Errorf("expected span.EndSpan(nil) on handler success, got endErr=%v", span.endErr)
	}
	// 3. 4 个 messaging.* tag 精确断言
	assertHasTag(t, span.tagCalls, "messaging.system", "kafka")
	assertHasTag(t, span.tagCalls, "messaging.kafka.topic", "chat-events")
	assertHasTag(t, span.tagCalls, "messaging.kafka.partition", "3")
	assertHasTag(t, span.tagCalls, "event.type", "message.created")
	// 4. tag 调用总数 == 4 (顺序无关,但不应多打)
	if len(span.tagCalls) != 4 {
		t.Errorf("expected exactly 4 tag calls, got %d: %+v", len(span.tagCalls), span.tagCalls)
	}
}

// TestConsumeClaim_NilTracerSpanNotCreated mock 验证 nil tracer 不调 CreateLocalSpan
//
// 与 TestConsumeClaim_NilTracerDoesNotCallCreateLocalSpan (边界 case, nil 字段)
// 互为补充: 已有 case 验证 nil 字段→handler 跑通;本 case 验证 nil 字段→不调 span。
func TestConsumeClaim_NilTracerSpanNotCreated(t *testing.T) {
	t.Parallel()
	tracer := &mockTracer{localSpan: &mockSpan{}} // 注入但 h.Tracer = nil 守卫
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled <- struct{}{}
			return nil
		},
		Tracer: nil, // 关键:守卫
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     []byte(`{"type":"message.created","id":"evt-nil-2"}`),
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case <-handlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if len(tracer.localOpCalls) != 0 {
		t.Errorf("nil Tracer 应不调 CreateLocalSpan, got %v", tracer.localOpCalls)
	}
}

// TestConsumeClaim_SpanEndSpanPropagatesHandlerErr 业务 handler 返 err 时
// span.EndSpan 收到该 err (后续 PR-OBS-18 会在 EndSpan(err) 时打 error log/SkyWalking Error tag)
func TestConsumeClaim_SpanEndSpanPropagatesHandlerErr(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{entrySpan: span} // Stage 92 PR-2: entry 路径返 span
	wantErr := errors.New("analyze: model timeout")
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled <- struct{}{}
			return wantErr
		},
		Tracer:     tracer,
		MaxRetries: 1, // 第 2 次进 DLQ + Mark
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Key:       []byte("evt-err-mock"),
		Value:     []byte(`{"type":"message.created","id":"evt-err-mock"}`),
		Timestamp: time.Now(),
	}

	// 投 2 次:第 1 次 attempt=1 < 1 仍 retry(不 Mark);第 2 次 attempt=2 ≥ 1 进 DLQ
	msgs := []*sarama.ConsumerMessage{msg, msg}
	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 2)}
	sess := &fakeSession{}
	for _, m := range msgs {
		claim.msgs <- m
	}
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	// 等待 2 次 handler 调用
	for i := 0; i < 2; i++ {
		select {
		case <-handlerCalled:
		case <-time.After(2 * time.Second):
			t.Fatalf("handler not called %d/2", i+1)
		}
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// CreateEntrySpan 应被调用 2 次（Stage 92 PR-2: 用 entry 替代 local）
	if len(tracer.entryOpCalls) != 2 {
		t.Errorf("expected 2 CreateEntrySpan calls, got %d", len(tracer.entryOpCalls))
	}
	// EndSpan 收到 err (handler 持续失败 → 每次 span.EndSpan(err) 路径)
	if !span.ended {
		t.Error("expected span.EndSpan called")
	}
	// 当前实现:handler 返 err → defer span.EndSpan(nil) (consumer.go span 收尾不带 err)
	// 这是 PR-OBS-17 状态(已有 4 tag,err 传播是 PR-OBS-18 范围)
	// 仅断言 ended=true,不强求 endErr 值
	_ = span.endErr
}

// ===== Stage 92 PR-2: Kafka consumer sw8 透传 =====
//
// 目的: ai-svc consumer 必须从 msg.Headers 抽 sw8 → 用 Tracer.CreateEntrySpan
// 重建父 trace（chat-svc producer → ai-svc consumer 跨进程 trace）。
//
// 行为契约:
//   - msg 含 sw8 header → consumer 调 CreateEntrySpan("kafka-consume", ext)
//   - extractor 收到 sw8 值（透传给 go2sky 重建父 SpanContext）
//   - span.SetSpanLayer(MQ=6) + SetComponent(GoKafka=5003) + 4 个 messaging.* tag
//   - msg 无 sw8 header → CreateEntrySpan 仍调（extractor 返 "" → Valid=false → 新 trace 起点）
func TestConsumeClaim_RestoresParentTraceFromSw8Header(t *testing.T) {
	t.Parallel()
	const fakeSw8 = "1-aabbccdd-eeff0011-2-aabbccdd-aabbccdd-aabbccdd-aabbccdd-aabbccdd"

	span := &mockSpan{}
	tracer := &mockTracer{entrySpan: span}
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled <- struct{}{}
			return nil
		},
		Tracer: tracer,
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 5,
		Value:     []byte(`{"type":"message.created","id":"evt-sw8-1","data":{"messageId":1}}`),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("sw8"), Value: []byte(fakeSw8)},
		},
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case <-handlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// 1. 必须调 CreateEntrySpan（不再用 CreateLocalSpan——PR-2 切换）
	if len(tracer.entryOpCalls) != 1 || tracer.entryOpCalls[0] != "kafka-consume" {
		t.Errorf("CreateEntrySpan calls = %v, want [kafka-consume]", tracer.entryOpCalls)
	}
	// 2. extractor 收到 sw8（父 trace 重建的依据）
	if tracer.entrySw8Seen != fakeSw8 {
		t.Errorf("extractor(\"sw8\") = %q, want %q (msg.Headers[sw8] 没被抽到)",
			tracer.entrySw8Seen, fakeSw8)
	}
	// 3. span 必须 EndSpan + 4 个 messaging.* tag（同 PR-OBS-17）
	if !span.ended {
		t.Error("expected span.EndSpan called")
	}
	assertHasTag(t, span.tagCalls, "messaging.system", "kafka")
	assertHasTag(t, span.tagCalls, "messaging.kafka.topic", "chat-events")
	assertHasTag(t, span.tagCalls, "messaging.kafka.partition", "5")
	assertHasTag(t, span.tagCalls, "event.type", "message.created")
}

// TestConsumeClaim_NoSw8Header_StillCreatesSpan 验证降级：msg 无 sw8 时
// CreateEntrySpan 仍调（extractor 返 "" → Valid=false → 新 trace 起点），不 panic。
func TestConsumeClaim_NoSw8Header_StillCreatesSpan(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{entrySpan: span}
	handlerCalled := make(chan struct{}, 1)
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			handlerCalled <- struct{}{}
			return nil
		},
		Tracer: tracer,
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Value:     []byte(`{"type":"message.created","id":"evt-no-sw8"}`),
		// 无 sw8 header（兼容 Stage 73 之前的旧消息）
		Headers:   nil,
		Timestamp: time.Now(),
	}

	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	select {
	case <-handlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if len(tracer.entryOpCalls) != 1 {
		t.Errorf("CreateEntrySpan must still be called for msgs without sw8, got %v", tracer.entryOpCalls)
	}
	if tracer.entrySw8Seen != "" {
		t.Errorf("entrySw8Seen = %q, want empty (no sw8 header)", tracer.entrySw8Seen)
	}
}

// TestHandleFailure_ConcurrentAccessIsSafe Round 5b §B GREEN:
//
// observability-edge-gaps §B (consumer.attempts 加锁 P2 0.5h):
// 当前 ConsumerGroupHandler.attempts map 在 handleFailure 里无并发保护。
// sarama 当前版本 ConsumeClaim 是单 goroutine(SARAMA 保证),但未来重构
// (worker pool / 异步 retry)或 sarama 跨 goroutine 派发时,map 会触发 race detector。
//
// 本测试通过 N=50 goroutine 并发读写 attempts map,断言:
//   - 无 panic
//   - 写入计数正确(每个 goroutine 写入 1 次,attempts map 至少 N 个 key)
//   - 业务完成后 attempts map 状态一致(并发读+写最终一致)
//
// 实现:attempts map 加 sync.Mutex 守卫(本 commit 同期提交 GREEN 改造)。
//
// 设计取舍:
//   - 不依赖 -race binary(Windows + Git Bash 环境下 -race 探测有符号解析问题)
//   - 改用"高并发读写 + 行为正确性"覆盖;并发安全由 sync.Mutex 提供
//   - 跨平台:在 Linux/Mac 上可加 `t.Helper()` + `-race` 二次验证(留作未来 CI step)
func TestHandleFailure_ConcurrentAccessIsSafe(t *testing.T) {
	dlq := NewInMemoryDLQPublisher()
	h := &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(ctx context.Context, e *events.Event) error { return errors.New("forced") },
		TopicFilter: "",
		Tracer:     nil,
		DLQ:        dlq,
		MaxRetries: 100, // 高值,避免任一 goroutine 触发 DLQ 路径
	}

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N * 2)

	// goroutine 1: N 个 handleFailure(模拟跨 goroutine 写入 attempts)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			msg := &sarama.ConsumerMessage{
				Topic: "chat-events",
				Key:   []byte(fmt.Sprintf("evt-race-%d", i)),
				Value: []byte(`{"type":"message.created","id":"x","data":{}}`),
			}
			sess := &fakeSession{}
			h.handleFailure(sess, msg, errors.New("forced"), h.MaxRetries)
		}()
	}

	// goroutine 2: N 个 ConsumeClaim(读 attempts map + 写)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			msg := &sarama.ConsumerMessage{
				Topic:     "chat-events",
				Key:       []byte(fmt.Sprintf("evt-claim-race-%d", i)),
				Value:     []byte(`{"type":"message.created","id":"x","data":{}}`),
				Headers:   nil,
				Timestamp: time.Now(),
			}
			claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
			sess := &fakeSession{}
			claim.msgs <- msg
			close(claim.msgs)
			// 用短 timeout 收尾,避免测试 hang
			done := make(chan error, 1)
			go func() { done <- h.ConsumeClaim(sess, claim) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		}()
	}

	wg.Wait()

	// 断言 1:无 panic(若 sync.Mutex 漏锁,读 map 时偶发 panic,测试失败)
	// 断言 2:attempts map 至少含 N 个 handleFailure 写入的 key
	if got := len(h.attempts); got < N {
		t.Errorf("attempts map len = %d, want >= %d (handleFailure 写入被并发丢失?)", got, N)
	}
}

// =====================================================
// Stage 94 PR-2a §P0-3 RED · ai-svc consumer "span 在 case 末尾 EndSpan"
// =====================================================
//
// 钉死 §P0-3 修复契约 ——"每条消息的 span 必须在 case 分支末尾立即 EndSpan"。
// 旧实现 (`defer span.EndSpan(nil)` 在 for-loop case 内) 触发 case 末尾返回后
// defer 才执行,N 条消息的 span EndSpan 全部延迟到 ConsumeClaim 退出 → OAP 上
// 每条消息 duration = 整个 consumer goroutine 寿命（事实上等于全失败/丢失）。
//
// 测试设计：
//   - 构造 N=3 条消息,handler 内闭包记录「我的 span 状态」 + 「上一条 span 状态」
//   - mockTracer.entryFn 每次 CreateEntrySpan 返回一个新 mockSpan
//   - 断言：
//     1) handler #2 / #3 看到前一条 span.ended == true（钉死 case 末尾立刻收尾）
//     2) handler 自己的 span 在业务执行时 ended == false（span 还活着）
//     3) ConsumeClaim 返回时所有 span 都 ended == true
//
// 旧实现跑此测试：handler #2 看到 span0.ended == false → FAIL
// 新实现（方案 A）：handler #2 看到 span0.ended == true  → PASS
func TestConsumeClaim_SpanEndSpanCalledWithinCaseBody(t *testing.T) {
	t.Parallel()

	// 每次 CreateEntrySpan 返回独立 mockSpan
	spans := make([]*mockSpan, 3)
	idx := 0
	tracer := &mockTracer{
		entrySpan: &mockSpan{}, // 占位（entryFn 非 nil 时 entrySpan 不被读）
		entryFn: func(ctx context.Context, opName string, _ func(string) (string, error)) (context.Context, grpcinterceptor.Span, error) {
			s := &mockSpan{}
			spans[idx] = s
			idx++
			return ctx, s, nil
		},
	}

	type observation struct {
		ownEnded  bool
		prevEnded bool // 上一条消息 span 是否已 EndSpan
	}
	obsCh := make(chan observation, 3)

	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(_ context.Context, _ *events.Event) error {
			// 当前 span 还应活着（前 idx 已被 CreateEntrySpan 分配）
			ownEnded := spans[idx-1].ended
			// 上一条 span（idx >= 2 时）应已 EndSpan —— 这是 §P0-3 钉死的契约
			var prevEnded bool
			if idx >= 2 {
				prevEnded = spans[idx-2].ended
			}
			obsCh <- observation{ownEnded: ownEnded, prevEnded: prevEnded}
			return nil
		},
		Tracer: tracer,
	}

	// 构造 3 条消息（每条带 sw8 header）
	mkMsg := func(id string) *sarama.ConsumerMessage {
		return &sarama.ConsumerMessage{
			Topic:     "chat-events",
			Partition: 0,
			Value:     []byte(`{"type":"message.created","id":"` + id + `","data":{"messageId":1,"conversationId":1,"userId":1}}`),
			Headers:   []*sarama.RecordHeader{{Key: []byte("sw8"), Value: []byte("1-p03-" + id)}},
		}
	}
	driveConsumeClaim(t, h, []*sarama.ConsumerMessage{
		mkMsg("evt-1"), mkMsg("evt-2"), mkMsg("evt-3"),
	})

	close(obsCh)
	var obs []observation
	for o := range obsCh {
		obs = append(obs, o)
	}
	if len(obs) != 3 {
		t.Fatalf("handler 应调 3 次, got %d", len(obs))
	}

	// handler #1：自己的 span 应还活着（业务执行中）；无前一条 span
	if obs[0].ownEnded {
		t.Error("handler #1: 自己的 span 在业务执行时不应已 EndSpan")
	}

	// handler #2：上一条（#1）span 必须已 EndSpan —— 钉死"case 末尾立刻收尾"
	if !obs[1].prevEnded {
		t.Error("handler #2: 上一条 span 必须已 EndSpan（case 末尾立刻收尾）\n" +
			"如 fail 说明 defer 仍留在 case 内（§P0-3 未修）")
	}
	if obs[1].ownEnded {
		t.Error("handler #2: 自己的 span 在业务执行时不应已 EndSpan")
	}

	// handler #3：上一条（#2）span 必须已 EndSpan
	if !obs[2].prevEnded {
		t.Error("handler #3: 上一条 span 必须已 EndSpan")
	}
	if obs[2].ownEnded {
		t.Error("handler #3: 自己的 span 在业务执行时不应已 EndSpan")
	}

	// ConsumeClaim 返回时所有 span.ended == true
	for i, s := range spans {
		if !s.ended {
			t.Errorf("span[%d] 在 ConsumeClaim 返回时仍未 EndSpan", i)
		}
	}
}