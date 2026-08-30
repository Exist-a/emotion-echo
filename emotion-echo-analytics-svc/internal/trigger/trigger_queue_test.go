// Package trigger — trigger_queue_test.go
//
// Sibling test for trigger_queue.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 3 part 2 RED: cover buffered channel + worker pool
// + backpressure (ErrQueueFull) + ctx cancel drain.
//
// Coverage matrix:
//
//   - Submit_HappyPath: worker 处理一个 Request
//   - Submit_QueueFull_Backpressure: cap=1 时塞第 2 个 → ErrQueueFull
//   - Submit_AfterClose_ReturnsErrQueueClosed
//   - Worker_ProcessesAllEnqueuedRequests
//   - Close_DrainsInFlightRequests
//   - Close_DoubleCloseIsNoop
//   - PendingCount_ReflectsChannelDepth
package trigger

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTriggerQueue_Submit_HappyPath 验证 worker 收到 Request 并调用 workerFn
func TestTriggerQueue_Submit_HappyPath(t *testing.T) {
	t.Parallel()
	var got Request
	processed := make(chan struct{})

	q := NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, req Request) {
		got = req
		close(processed)
	})
	defer q.Close(context.Background())

	err := q.Submit(Request{UserID: 42, AssessmentType: "daily", TraceID: "trace-1"})
	require.NoError(t, err)

	select {
	case <-processed:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not process request within 500ms")
	}
	assert.Equal(t, int64(42), got.UserID)
	assert.Equal(t, "daily", got.AssessmentType)
}

// TestTriggerQueue_Submit_QueueFull_Backpressure 验证 cap=1 塞第 2 个返 ErrQueueFull
func TestTriggerQueue_Submit_QueueFull_Backpressure(t *testing.T) {
	t.Parallel()
	// worker 阻塞在 workerFn 内（不消费 channel）
	block := make(chan struct{})
	q := NewTriggerQueue(context.Background(), 1, 1, func(_ context.Context, _ Request) {
		<-block
	})
	defer func() {
		close(block)
		q.Close(context.Background())
	}()

	// 第 1 个 Request 进入 cap=1 channel（成功），worker 立刻占用
	require.NoError(t, q.Submit(Request{UserID: 1}))
	// 第 2 个 Request 应触发 backpressure
	err := q.Submit(Request{UserID: 2})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueFull)
}

// TestTriggerQueue_Submit_AfterClose_ReturnsErrQueueClosed
func TestTriggerQueue_Submit_AfterClose_ReturnsErrQueueClosed(t *testing.T) {
	t.Parallel()
	q := NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ Request) {})
	require.NoError(t, q.Close(context.Background()))

	err := q.Submit(Request{UserID: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueClosed)
}

// TestTriggerQueue_Worker_ProcessesAllEnqueuedRequests 验证 N 个 Request 都被处理
func TestTriggerQueue_Worker_ProcessesAllEnqueuedRequests(t *testing.T) {
	t.Parallel()
	var counter int64

	q := NewTriggerQueue(context.Background(), 2, 8, func(_ context.Context, _ Request) {
		atomic.AddInt64(&counter, 1)
	})

	const N = 5
	for i := 0; i < N; i++ {
		require.NoError(t, q.Submit(Request{UserID: int64(i)}))
	}

	// 等待所有 Request 处理完
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&counter) == N
	}, 500*time.Millisecond, 10*time.Millisecond, "expected %d processed, got %d", N, atomic.LoadInt64(&counter))

	require.NoError(t, q.Close(context.Background()))
}

// TestTriggerQueue_Close_DrainsInFlightRequests 验证 Close 等待 worker 排空
func TestTriggerQueue_Close_DrainsInFlightRequests(t *testing.T) {
	t.Parallel()
	var processed int64
	q := NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ Request) {
		time.Sleep(20 * time.Millisecond) // 模拟 work
		atomic.AddInt64(&processed, 1)
	})

	for i := 0; i < 3; i++ {
		require.NoError(t, q.Submit(Request{UserID: int64(i)}))
	}
	require.NoError(t, q.Close(context.Background()))
	assert.Equal(t, int64(3), atomic.LoadInt64(&processed), "Close 应等待所有 in-flight Request 处理完")
}

// TestTriggerQueue_Close_DoubleCloseIsNoop
func TestTriggerQueue_Close_DoubleCloseIsNoop(t *testing.T) {
	t.Parallel()
	q := NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ Request) {})
	require.NoError(t, q.Close(context.Background()))
	// 第二次 close 不应 panic 或返 error
	require.NoError(t, q.Close(context.Background()))
}

// TestTriggerQueue_PendingCount_ReflectsChannelDepth
func TestTriggerQueue_PendingCount_ReflectsChannelDepth(t *testing.T) {
	t.Parallel()
	// Worker 阻塞在 release channel 上 → 第一个 Submit 进入后
	// worker 立刻占用 (但不再消费)。后续 Submit 进入 channel buffer。
	release := make(chan struct{})

	q := NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ Request) {
		<-release // 阻塞 worker 消费后续 message
	})
	defer func() {
		close(release)
		q.Close(context.Background())
	}()

	require.NoError(t, q.Submit(Request{UserID: 1}))
	// 等 worker goroutine 启动并取走第 1 个
	require.Eventually(t, func() bool {
		return q.PendingCount() == 0
	}, 200*time.Millisecond, 5*time.Millisecond)

	require.NoError(t, q.Submit(Request{UserID: 2}))
	require.NoError(t, q.Submit(Request{UserID: 3}))
	assert.Equal(t, 2, q.PendingCount(), "还剩 2 个在 channel 里")
}

// TestTriggerQueue_Submit_ConcurrentNoRace 验证 Submit 并发安全
func TestTriggerQueue_Submit_ConcurrentNoRace(t *testing.T) {
	t.Parallel()
	q := NewTriggerQueue(context.Background(), 4, 64, func(_ context.Context, _ Request) {})
	defer q.Close(context.Background())

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			err := q.Submit(Request{UserID: int64(i)})
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}