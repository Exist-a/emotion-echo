// Package trigger — trigger_queue.go
//
// Stage 30-A Round 3 part 2: TriggerQueue — buffered channel + backpressure
// goroutine worker pool for async mental_health assessment jobs.
//
// 契约（per docs/stage-30-A §三.2）：
//   - buffered channel cap=64
//   - Submit returns ErrQueueFull when channel is full (backpressure)
//   - worker pool drains channel and runs TriggerAssessment
//   - worker pool ctx cancel → graceful drain
package trigger

import (
	"context"
	"errors"
	"log"
	"sync"
)

// Request 触发评估请求
//
// 由 main.go 或 handler 提交；worker 异步执行。
type Request struct {
	UserID         int64  // 用户 ID
	AssessmentType string // daily|weekly|comprehensive
	TraceID        string // 链路追踪
}

// ErrQueueFull channel 已满时返回（backpressure）
var ErrQueueFull = errors.New("trigger queue is full")

// ErrQueueClosed queue 已关闭时返回
var ErrQueueClosed = errors.New("trigger queue is closed")

// DefaultQueueCap 默认 channel buffer 大小
const DefaultQueueCap = 64

// TriggerQueue buffered channel + worker pool
type TriggerQueue struct {
	ch       chan Request
	wg       sync.WaitGroup
	closed   chan struct{}
	closeMu  sync.RWMutex
	workerFn func(ctx context.Context, req Request)
}

// NewTriggerQueue 构造 channel + 启动 worker pool
//
// workers: 并发 worker 数（推荐 = GOMAXPROCS；<1 用 1；= 0 不启动
//          worker — 仅用于 buffer-only 场景如 backpressure 测试）
// cap: channel buffer（<=0 用 DefaultQueueCap）
func NewTriggerQueue(ctx context.Context, workers, cap int, workerFn func(ctx context.Context, req Request)) *TriggerQueue {
	if cap <= 0 {
		cap = DefaultQueueCap
	}
	if workers < 0 {
		workers = 0
	}

	q := &TriggerQueue{
		ch:       make(chan Request, cap),
		closed:   make(chan struct{}),
		workerFn: workerFn,
	}

	// workers == 0 保留：测试 backpressure 用，不启动 worker 协程
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.workerLoop(ctx)
	}

	return q
}

func (q *TriggerQueue) workerLoop(ctx context.Context) {
	defer q.wg.Done()
	for {
		// 优先消费 channel（保证 Close() 排空）
		select {
		case req, ok := <-q.ch:
			if !ok {
				return // channel 已关闭且排空
			}
			q.workerFn(ctx, req)
			continue
		default:
		}
		// channel 空 → 等待新数据 / 关闭 / ctx cancel
		select {
		case <-ctx.Done():
			return
		case <-q.closed:
			// 收到关闭信号 — 排空剩余 channel 再退出
			for {
				req, ok := <-q.ch
				if !ok {
					return
				}
				q.workerFn(ctx, req)
			}
		case req, ok := <-q.ch:
			if !ok {
				return
			}
			q.workerFn(ctx, req)
		}
	}
}

// Submit 提交一个 Request
//
//   - queue 已关闭 → ErrQueueClosed
//   - channel 已满 → ErrQueueFull（caller 决定退避重试 or 503）
//   - 否则入队成功，返回 nil
func (q *TriggerQueue) Submit(req Request) error {
	q.closeMu.RLock()
	defer q.closeMu.RUnlock()
	select {
	case <-q.closed:
		return ErrQueueClosed
	default:
	}
	select {
	case q.ch <- req:
		return nil
	default:
		return ErrQueueFull
	}
}

// Close 关闭 queue，等待 worker 排空已入队的 request
//
//   - 标记 closed（之后 Submit 返 ErrQueueClosed）
//   - 关闭 channel（worker 排空已入队的后退出）
//   - 等待所有 worker 退出（带 ctx timeout）
func (q *TriggerQueue) Close(ctx context.Context) error {
	q.closeMu.Lock()
	select {
	case <-q.closed:
		q.closeMu.Unlock()
		return nil // 重复 close 是 no-op
	default:
	}
	close(q.closed)
	close(q.ch)
	q.closeMu.Unlock()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		log.Printf("[trigger] close timeout: %v", ctx.Err())
		return ctx.Err()
	}
}

// PendingCount 当前 channel 内等待的 Request 数（仅用于 metrics）
func (q *TriggerQueue) PendingCount() int {
	return len(q.ch)
}