// Package fusion — Stage 35 · PR-5 GREEN
//
// CircuitBreaker 三态熔断器：Closed / Open / HalfOpen
//
// 设计（ADR-15 §E）：
//   - Closed：正常调底层；累计连续 FailThreshold 次失败 → Open
//   - Open：拒绝所有调用，立即返回 ErrCircuitOpen；Open 持续 OpenSeconds 后转 HalfOpen
//   - HalfOpen：允许 1 次试探调用；成功 → Closed；失败 → Open（重新计时）
//
// 线程安全：sync.Mutex
//
// 为什么用熔断不用重试：
//   - retry 会让 Worker 任务堆积；熔断是"早打" + "少打"，更节能
//   - Stage 35 目标是"防雪崩"，不是"反复打"
package fusion

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen 熔断器 Open 时返回的错误。
var ErrCircuitOpen = errors.New("circuit breaker is open")

// BreakerState 熔断器状态。
type BreakerState int

const (
	BreakerClosed BreakerState = iota
	BreakerHalfOpen
	BreakerOpen
)

// String 实现 fmt.Stringer。
func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerHalfOpen:
		return "half_open"
	case BreakerOpen:
		return "open"
	default:
		return "unknown"
	}
}

// BreakerConfig 熔断器配置。
type BreakerConfig struct {
	FailThreshold int           // 连续失败次数阈值（默认 5）
	OpenSeconds   time.Duration // Open 持续时间（默认 30s）
}

// CircuitBreaker 三态熔断器。
type CircuitBreaker struct {
	mu sync.Mutex

	state          BreakerState
	consecFails    int       // 连续失败计数（Closed 状态）
	openedAt       time.Time // 进入 Open 的时刻
	halfOpenInUse  bool      // HalfOpen 时是否已发过 1 次试探

	failThreshold int
	openSeconds   time.Duration

	// nowFunc 用于测试注入（默认 time.Now）。
	nowFunc func() time.Time
}

// NewCircuitBreaker 构造器。
func NewCircuitBreaker(cfg BreakerConfig) *CircuitBreaker {
	threshold := cfg.FailThreshold
	if threshold <= 0 {
		threshold = 5
	}
	openSecs := cfg.OpenSeconds
	if openSecs <= 0 {
		openSecs = 30 * time.Second
	}
	return &CircuitBreaker{
		state:         BreakerClosed,
		failThreshold: threshold,
		openSeconds:   openSecs,
		nowFunc:       time.Now,
	}
}

// State 当前状态。
func (b *CircuitBreaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionIfNeeded()
	return b.state
}

// Allow 询问是否可以发起调用。
//
// 返回 true → 允许；false → 拒绝（Open 状态）。
// HalfOpen 时只允许 1 次试探。
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionIfNeeded()

	switch b.state {
	case BreakerClosed:
		return true
	case BreakerHalfOpen:
		if b.halfOpenInUse {
			return false
		}
		b.halfOpenInUse = true
		return true
	case BreakerOpen:
		return false
	default:
		return false
	}
}

// RecordSuccess 记录一次成功（Closed 重置计数；HalfOpen → Closed）。
func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecFails = 0
	if b.state == BreakerHalfOpen {
		b.state = BreakerClosed
		b.halfOpenInUse = false
	}
}

// RecordFailure 记录一次失败（Closed 累计；HalfOpen → Open）。
func (b *CircuitBreaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == BreakerHalfOpen {
		// 试探失败 → 重新 Open（计时重置）
		b.state = BreakerOpen
		b.openedAt = b.nowFunc()
		b.halfOpenInUse = false
		return
	}

	b.consecFails++
	if b.state == BreakerClosed && b.consecFails >= b.failThreshold {
		b.state = BreakerOpen
		b.openedAt = b.nowFunc()
	}
}

// RecordResult 便捷方法，按 err 是否为 nil 决定记 success 还是 failure。
func (b *CircuitBreaker) RecordResult(err error) {
	if err != nil {
		b.RecordFailure()
		return
	}
	b.RecordSuccess()
}

// transitionIfNeeded 检查 Open 是否到期（需持锁）。
func (b *CircuitBreaker) transitionIfNeeded() {
	if b.state == BreakerOpen && b.nowFunc().Sub(b.openedAt) >= b.openSeconds {
		b.state = BreakerHalfOpen
		b.halfOpenInUse = false
	}
}