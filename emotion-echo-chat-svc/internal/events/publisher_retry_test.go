package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== CL-03 Phase 5（CL-D-19=C / CL-F-15）：Kafka 未就绪时不再"静默改投内存" =====
//
// 修复前：producer 初始化失败 ⇒ `pub = InMemoryEventPublisher` ⇒ relay 每轮把 outbox 行
// 标 `sent`，而事件只进进程内存（B 轨实测 CL-F-15：topic 里没有该事件、消费端也没收到，
// 唯一暴露它的是一个 counter）。
//
// 修复后：① 初始化失败 ⇒ 装 `UnavailablePublisher`（Publish 一律报错）⇒ relay **不会**
// MarkSent（行留 pending）；② 后台指数退避重试，成功后热替换成真 publisher ⇒ 行自然重发。

// TestUnavailablePublisher_AlwaysErrors 契约：一律报错且错误可识别。
func TestUnavailablePublisher_AlwaysErrors(t *testing.T) {
	p := NewUnavailablePublisher(errors.New("dial tcp: connection refused"))

	err := p.Publish(context.Background(), "chat-events", &Event{ID: "e1", Type: "message.created"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPublisherUnavailable),
		"错误必须可被 errors.Is(ErrPublisherUnavailable) 识别 —— relay 靠它区分「未就绪」与「发布失败」")
	assert.Contains(t, err.Error(), "connection refused", "原始原因应保留（便于排查）")
	assert.NoError(t, p.Close())
}

// TestUnavailablePublisher_NilReason 不传原因也要可用。
func TestUnavailablePublisher_NilReason(t *testing.T) {
	p := NewUnavailablePublisher(nil)
	err := p.Publish(context.Background(), "t", &Event{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPublisherUnavailable))
}

// TestPublisherHolder_SetGet_RaceFree 并发 Set/Get 不得数据竞争（-race 下跑）。
func TestPublisherHolder_SetGet_RaceFree(t *testing.T) {
	h := NewPublisherHolder(NewInMemoryEventPublisher())
	require.NotNil(t, h.Get())

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				h.Set(NewInMemoryEventPublisher())
				_ = h.Get()
			}
		}()
	}
	wg.Wait()
	assert.NotNil(t, h.Get())
}

// TestPublisherHolder_Set_DifferentConcreteTypes_NoPanic ★ CL-03 收口运行时实拍的缺陷 ★
//
// 生产装配序列是「init 失败先装 `*UnavailablePublisher` → 后台重试成功 → 热替换成
// `*KafkaEventPublisher`」，这是**两个不同具体类型**。`atomic.Value` 要求同一个 Value
// 始终存同一具体类型，否则 **panic: sync/atomic: store of inconsistently typed value into Value**。
//
// 起栈实测（2026-10-16 14:02:56）：Kafka 恢复瞬间 chat-svc **直接 panic 退出**、容器重启
// （`RestartCount=1`），行虽被重启后的新进程补发，但"热替换"路径实际从未走通。
func TestPublisherHolder_Set_DifferentConcreteTypes_NoPanic(t *testing.T) {
	h := NewPublisherHolder(nil)

	require.NotPanics(t, func() {
		h.Set(NewUnavailablePublisher(errors.New("init failed")))
	}, "装配 UnavailablePublisher 不得 panic")

	require.NotPanics(t, func() {
		h.Set(NewInMemoryEventPublisher())
	}, "热替换成另一种具体类型不得 panic —— 生产里这就是 Kafka 恢复时的热替换路径")

	got := h.Get()
	require.NotNil(t, got, "热替换后 Get 应返回新 publisher，而不是 nil")
	assert.IsType(t, &InMemoryEventPublisher{}, got, "Get 应返回最后一次 Set 的那个 publisher")
}

// TestPublisherHolder_EmptyReturnsNil 未设置时返回 nil（调用方据此判"未就绪"）。
func TestPublisherHolder_EmptyReturnsNil(t *testing.T) {
	h := &PublisherHolder{}
	assert.Nil(t, h.Get())
}

// TestStartKafkaPublisherBackgroundRetry_SucceedsAfterFailures 失败 N 次后成功 ⇒ 调 OnSuccess 且停止重试。
func TestStartKafkaPublisherBackgroundRetry_SucceedsAfterFailures(t *testing.T) {
	tries := 0
	var got *KafkaEventPublisher
	done := make(chan struct{})

	stop := StartKafkaPublisherBackgroundRetry(context.Background(), KafkaPublisherRetryOptions{
		InitialInterval: time.Millisecond,
		MaxInterval:     2 * time.Millisecond,
		Try: func() (*KafkaEventPublisher, error) {
			tries++
			if tries < 3 {
				return nil, errors.New("kafka not ready")
			}
			return &KafkaEventPublisher{}, nil
		},
		OnSuccess: func(kp *KafkaEventPublisher) {
			got = kp
			close(done)
		},
	})
	defer stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("重试未在预期时间内成功")
	}
	assert.Equal(t, 3, tries, "应第 3 次成功")
	assert.NotNil(t, got, "OnSuccess 必须拿到成功构造的 publisher")
}

// TestStartKafkaPublisherBackgroundRetry_StopsOnCtxCancel ctx 取消 ⇒ 循环退出（stop 不阻塞）。
func TestStartKafkaPublisherBackgroundRetry_StopsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tries := 0
	var mu sync.Mutex

	stop := StartKafkaPublisherBackgroundRetry(ctx, KafkaPublisherRetryOptions{
		InitialInterval: time.Millisecond,
		MaxInterval:     2 * time.Millisecond,
		Try: func() (*KafkaEventPublisher, error) {
			mu.Lock()
			tries++
			mu.Unlock()
			return nil, errors.New("still down")
		},
		OnSuccess: func(*KafkaEventPublisher) { t.Error("不应成功") },
	})

	time.Sleep(30 * time.Millisecond)
	cancel()
	stop() // 不得阻塞

	mu.Lock()
	n := tries
	mu.Unlock()
	assert.Positive(t, n, "取消前应至少尝试过一次")
}

// TestStartKafkaPublisherBackgroundRetry_NilOnSuccess_NoPanic 未提供 OnSuccess 也不得 panic。
func TestStartKafkaPublisherBackgroundRetry_NilOnSuccess_NoPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stop := StartKafkaPublisherBackgroundRetry(ctx, KafkaPublisherRetryOptions{
		InitialInterval: time.Millisecond,
		MaxInterval:     time.Millisecond,
		Try:             func() (*KafkaEventPublisher, error) { return &KafkaEventPublisher{}, nil },
	})
	time.Sleep(20 * time.Millisecond)
	cancel()
	require.NotPanics(t, stop)
}
