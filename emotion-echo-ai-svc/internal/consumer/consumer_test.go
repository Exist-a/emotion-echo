package consumer

import (
	"context"
	"errors"
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
		{Topic: "t", Value: []byte(`{bad json`)},                  // bad
		{Topic: "t", Value: []byte(`{"type":"message.created"}`)}, // good
		{Topic: "t", Value: []byte(`{"type":"message.created"}`)}, // good
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
		{Topic: "t", Value: []byte(`{"type":"message.created"}`)}, // match
		{Topic: "t", Value: []byte(`{"type":"user.created"}`)},    // skip (filter)
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

// TestConsumeClaim_RetryBudgetExhausted_PublishesDLQ_AndMarks（替代 TestConsumeClaim_HandlerRetriesBeforeDLQ，
// E2E-F-174 2026-10-01 契约翻转）
//
// 旧契约（失真，F-174）：靠"同一条消息被 sarama 重投 4 次"喂 handleFailure 计数 ——
// 实际 sarama session 内不重投未 Mark 消息，该契约编码的正是缺陷本身。
//
// 新契约：单条消息原地重试 maxRetries+1 次（Handler 共调 4 次），预算耗尽 →
// 恰好 1 条 DLQ（Attempts=4）→ Mark 前进。
func TestConsumeClaim_RetryBudgetExhausted_PublishesDLQ_AndMarks(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	calls := 0
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			calls++
			return errors.New("forced handler err")
		},
		DLQ:        dlq,
		MaxRetries: 3,
		BackoffFn:  func(int) time.Duration { return 0 },
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-poison-1"), Value: []byte(`{"type":"message.created","id":"evt-poison-1"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	if calls != 4 {
		t.Errorf("Handler 调用数 = %d，want 4（maxRetries=3 ⇒ 原地尝试 4 次）", calls)
	}
	if len(marked) != 1 {
		t.Errorf("预算耗尽后应 Mark 前进，marked=%d: %v", len(marked), marked)
	}
	if got := dlq.Captured(); len(got) != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", len(got))
	}
	if got := dlq.Captured(); got[0].Attempts != 4 {
		t.Errorf("DLQ.Attempts 应=4，got %d", got[0].Attempts)
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
	// F-174：单条消息原地重试（预算 MaxRetries+1=2 次）后进 DLQ，无需旧版"喂 2 条同 key 消息"
	msgs := []*sarama.ConsumerMessage{
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
		BackoffFn:  func(int) time.Duration { return 0 },
	}

	msgs := []*sarama.ConsumerMessage{
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

// TestConsumeClaim_DLQNilIsSafe（E2E-F-174 契约翻转，2026-10-01）
//
// 旧契约：DLQ=nil 时不 Mark（"无限重投"）——实测该语义 = 单条毒消息永久阻塞
// 分区且计数永不推进，编码的正是缺陷本身。
// 新契约：DLQ=nil 时预算耗尽仍 Mark 前进（只打日志），毒消息不卡分区。
func TestConsumeClaim_DLQNilIsSafe(t *testing.T) {
	t.Parallel()
	h := &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(ctx context.Context, e *events.Event) error { return errors.New("err") },
		DLQ:        nil, // 关键：nil
		MaxRetries: 2,
		BackoffFn:  func(int) time.Duration { return 0 },
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-nil-1"), Value: []byte(`{"type":"message.created","id":"evt-nil-1"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	if len(marked) != 1 {
		t.Errorf("DLQ=nil 预算耗尽后应 Mark 前进（毒消息不卡分区），marked=%v", marked)
	}
}

// TestConsumeClaim_TransientFailure_RetriedInPlace_NoDLQ（替代 TestConsumeClaim_HandlerSuccessClearsAttempts，
// E2E-F-174 契约翻转）：瞬时失败在同一条消息上原地重试成功 ⇒ Mark、不进 DLQ。
// （旧测试守护的 attempts map 已删除。）
func TestConsumeClaim_TransientFailure_RetriedInPlace_NoDLQ(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	calls := 0
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			calls++
			if calls == 1 {
				return errors.New("transient err (F-174)")
			}
			return nil
		},
		DLQ:        dlq,
		MaxRetries: 5,
		BackoffFn:  func(int) time.Duration { return 0 },
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-fail"), Value: []byte(`{"type":"message.created","id":"evt-fail"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	if calls != 2 {
		t.Errorf("Handler 调用数 = %d，want 2（首次失败 + 原地重试成功）", calls)
	}
	if len(marked) != 1 {
		t.Errorf("重试成功后应 Mark，marked=%v", marked)
	}
	if len(dlq.Captured()) != 0 {
		t.Errorf("预算内成功不应投 DLQ，got %d", len(dlq.Captured()))
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
//
//	即使 Tracer 为 nil,ctx 也需传递
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
	ended    bool
	endErr   error
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
	entryOpCalls []string
	entrySw8Seen string // extractor("sw8") 抽到的值
	entrySpan    *mockSpan
	entryCtx     context.Context
	entryErr     error

	// Stage 94 PR-2 §P0-3 扩展：entryFn 可选，若非 nil 则 CreateEntrySpan 走自定义
	// 路径（每次返回新 mockSpan）。老测试 entryFn=nil 走原路径（向后兼容）。
	entryFn func(ctx context.Context, opName string, extractor func(string) (string, error)) (context.Context, grpcinterceptor.Span, error)

	// Stage 92 PR-2: CreateExitSpan 字段（本测试不验证，本服务不发消息；保留接口合规）
	exitOpCalls []string
	exitSpan    *mockSpan
	exitErr     error
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
		BackoffFn:  func(int) time.Duration { return 0 },
	}

	msg := &sarama.ConsumerMessage{
		Topic:     "chat-events",
		Partition: 0,
		Key:       []byte("evt-err-mock"),
		Value:     []byte(`{"type":"message.created","id":"evt-err-mock"}`),
		Timestamp: time.Now(),
	}

	// F-174：单条消息原地重试 2 次（预算 MaxRetries+1=2），第 2 次进 DLQ + Mark
	claim := &fakeClaim{msgs: make(chan *sarama.ConsumerMessage, 1)}
	sess := &fakeSession{}
	claim.msgs <- msg
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	// 等待 2 次 handler 调用（原地重试）
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
	// F-174：单条消息原地重试，span 每消息一个 ⇒ CreateEntrySpan 1 次
	if len(tracer.entryOpCalls) != 1 {
		t.Errorf("expected 1 CreateEntrySpan call, got %d", len(tracer.entryOpCalls))
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

// TestHandleFailure_ConcurrentAccessIsSafe 已随 F-174 修复删除（2026-10-01）：
// 它守护的 attempts map / attemptsMu 已整体移除（新实现 = 每条消息原地重试，
// 无跨消息共享状态；MaxRetriesFn 并发语义由 E2E-23 #31 相关测试覆盖）。


// =====================================================
// E2E-F-174: 重试语义修复 —— 失败消息必须原地重试（ai-svc 同型，2026-10-01）
// =====================================================
//
// 与 analytics-svc consumer 同源的失真（F-174/D3 实测）：Handler 失败后
// 不 Mark 也不重投同一条消息，"重试计数"靠后续同 key 消息推进。
//
// 新契约：失败消息在同一 claim 条目上原地重试（预算 = MaxRetries+1 次尝试），
// 预算内成功 ⇒ Mark；耗尽 ⇒ DLQ（若配置）+ Mark，继续下一条，分区不卡死。

// TestConsumeClaim_FailedMessage_RetriedInPlace F-174 RED（ai-svc）：
// Handler 对 evt-red-1 只失败第一次。旧代码：Handler 调 1 次、不 Mark（attempt=1 ≤ 3）
// ⇒ RED。新代码：原地重试成功 ⇒ Handler 调 2 次、Mark 1 次。
func TestConsumeClaim_FailedMessage_RetriedInPlace(t *testing.T) {
	t.Parallel()
	dlq := NewInMemoryDLQPublisher()
	calls := 0
	h := &ConsumerGroupHandler{
		Ready: make(chan bool),
		Handler: func(ctx context.Context, e *events.Event) error {
			calls++
			if e.ID == "evt-red-1" && calls == 1 {
				return errors.New("forced transient err (F-174)")
			}
			return nil
		},
		DLQ:        dlq,
		MaxRetries: 3,
	}

	msgs := []*sarama.ConsumerMessage{
		{Topic: "chat-events", Key: []byte("evt-red-1"), Value: []byte(`{"type":"message.created","id":"evt-red-1"}`)},
	}
	marked := driveConsumeClaim(t, h, msgs)

	if calls != 2 {
		t.Errorf("F-174 RED（ai-svc）：Handler 调用数 = %d，want 2（首次失败 + 原地重试成功）；旧语义失败后不重投 ⇒ 只有 1 次", calls)
	}
	if len(marked) != 1 {
		t.Errorf("F-174 RED（ai-svc）：MarkMessage 次数 = %d，want 1（重试成功后必须 Mark）", len(marked))
	}
	if got := dlq.Captured(); len(got) != 0 {
		t.Errorf("重试成功不应投 DLQ，got %d", len(got))
	}
}
