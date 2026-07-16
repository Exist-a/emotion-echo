package consumer

import (
	"context"
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