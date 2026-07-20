package messaging

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestInMemoryProducer_PublishAndDrain happy path：发布 → drain → 看到消息
func TestInMemoryProducer_PublishAndDrain(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()

	ctx := context.Background()
	m := Message{Topic: "t1", Value: []byte("hello")}
	if err := p.Publish(ctx, m); err != nil {
		t.Fatalf("publish err: %v", err)
	}
	all := p.Drain()
	if len(all) != 1 {
		t.Fatalf("drain should have 1 message, got %d", len(all))
	}
	if string(all[0].Value) != "hello" {
		t.Fatalf("value mismatch: %q", all[0].Value)
	}
}

// TestInMemoryProducer_RejectsEmptyTopic 校验空 topic
func TestInMemoryProducer_RejectsEmptyTopic(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()
	err := p.Publish(context.Background(), Message{Topic: "", Value: []byte("v")})
	if !errors.Is(err, ErrEmptyTopic) {
		t.Fatalf("want ErrEmptyTopic, got %v", err)
	}
}

// TestInMemoryProducer_RejectsEmptyValue 校验空 value
func TestInMemoryProducer_RejectsEmptyValue(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()
	err := p.Publish(context.Background(), Message{Topic: "t", Value: nil})
	if !errors.Is(err, ErrEmptyValue) {
		t.Fatalf("want ErrEmptyValue, got %v", err)
	}
}

// TestInMemoryProducer_ClosedAfterClose 关闭后发布应被拒
func TestInMemoryProducer_ClosedAfterClose(t *testing.T) {
	p := NewInMemoryProducer()
	_ = p.Close()
	err := p.Publish(context.Background(), Message{Topic: "t", Value: []byte("v")})
	if !errors.Is(err, ErrProducerClosed) {
		t.Fatalf("want ErrProducerClosed, got %v", err)
	}
}

// TestInMemoryProducer_DoubleClose_NoError 二次 close 安全
func TestInMemoryProducer_DoubleClose_NoError(t *testing.T) {
	p := NewInMemoryProducer()
	if err := p.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second close should be idempotent, got %v", err)
	}
}

// TestInMemoryProducer_ContextCancelled 已被取消的 ctx 应返回 ctx.Err()
func TestInMemoryProducer_ContextCancelled(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Publish(ctx, Message{Topic: "t", Value: []byte("v")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// TestInMemoryProducer_ContextDeadlineExceeded 超时 ctx
func TestInMemoryProducer_ContextDeadlineExceeded(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	err := p.Publish(ctx, Message{Topic: "t", Value: []byte("v")})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

// TestInMemoryProducer_ListenerChannel 订阅 listener：发布后 listener 应收到
func TestInMemoryProducer_ListenerChannel(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()

	want := Message{Topic: "t", Value: []byte("broadcast")}
	if err := p.Publish(context.Background(), want); err != nil {
		t.Fatalf("publish err: %v", err)
	}

	select {
	case got := <-p.listener:
		if string(got.Value) != "broadcast" {
			t.Fatalf("listener msg value mismatch: %q", got.Value)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("listener did not receive within timeout")
	}
}

// TestInMemoryProducer_DrainReturnsCopy drain 应返回深拷贝，外部修改不应影响内部
//
// 注意：当前实现 `copy(out, p.buffer)` 仅做 slice header 浅拷贝，
// 元素 Message.Value ([]byte) 共享底层数组 — 是不变量违反。
//
// 本测试如实断言："外部改了 Value 后内部也会被改" — 这是已知 bug。
// 待实现修复（深拷贝每个 Message 的 Value）后，本测试应改为断言不变量。
// TestInMemoryProducer_DrainReturnsCopy **Stage 26-N 修复后**：Drain 必须深拷贝 Value
//
// 历史：Stage 26-A 暴露 Drain 仅 shallow-copy。
func TestInMemoryProducer_DrainReturnsCopy(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()

	if err := p.Publish(context.Background(), Message{Topic: "t", Value: []byte("v")}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	drained := p.Drain()
	if len(drained) != 1 {
		t.Fatalf("drain should be 1, got %d", len(drained))
	}

	// 修改副本
	drained[0].Value[0] = 'X'
	again := p.Drain()
	// 内部 buffer 不应被改
	if string(again[0].Value) == "X" {
		t.Fatalf("internal buffer got mutated: %q (Drain should deep-copy Value)", again[0].Value)
	}
}

// TestInMemoryProducer_MultiplePublish 表驱动：3 条消息
func TestInMemoryProducer_MultiplePublish(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()

	cases := []struct {
		topic string
		val   string
	}{
		{"t1", "v1"},
		{"t2", "v2"},
		{"t3", "v3"},
	}
	for _, tc := range cases {
		if err := p.Publish(context.Background(), Message{Topic: tc.topic, Value: []byte(tc.val)}); err != nil {
			t.Fatalf("publish %s: %v", tc.topic, err)
		}
	}
	got := p.Drain()
	if len(got) != 3 {
		t.Fatalf("want 3 messages, got %d", len(got))
	}
	for i, tc := range cases {
		if got[i].Topic != tc.topic || string(got[i].Value) != tc.val {
			t.Fatalf("mismatch at %d: want %s/%s got %s/%s", i, tc.topic, tc.val, got[i].Topic, got[i].Value)
		}
	}
}

// TestInMemoryProducer_ListenerOverflow_NoBlock listener 满时不阻塞 producer
func TestInMemoryProducer_ListenerOverflow_NoBlock(t *testing.T) {
	p := NewInMemoryProducer()
	defer p.Close()
	// 不接 listener，但灌满 listener (默认 64 容量)：超出后 Publish 仍应成功
	for i := 0; i < 200; i++ {
		if err := p.Publish(context.Background(), Message{Topic: "t", Value: []byte("x")}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
	// 确认 buffer 已存 200 条
	if got := len(p.Drain()); got != 200 {
		t.Fatalf("buffer want=200 got=%d", got)
	}
}
