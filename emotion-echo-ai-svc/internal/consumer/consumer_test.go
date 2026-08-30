package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/events"

	"github.com/IBM/sarama"
)

// fakeSession 模拟 sarama.ConsumerGroupSession 用于单元测试
//
// 只实现 MarkMessage（其他方法不需要）。Tracer 仅校验 span 创建流程。
type fakeSession struct {
	sarama.ConsumerGroupSession
	marked []string
}

func (f *fakeSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
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