// Package fusion — Stage 35 · PR-5 RED
//
// CircuitBreaker 三态熔断器：Closed / Open / HalfOpen
//
// 设计：
//   - Closed → 累计连续 FailThreshold 次失败进入 Open
//   - Open   → 拒绝所有调用，立即返回 ErrCircuitOpen；Open 持续 OpenSeconds 后进入 HalfOpen
//   - HalfOpen → 允许 1 次试探调用：成功 → Closed；失败 → Open（重新计时）
//
// 线程安全：sync.Mutex
package fusion

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBreaker_ClosedAllowsCalls 初始 Closed 状态允许调用。
func TestBreaker_ClosedAllowsCalls(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 3, OpenSeconds: 30 * time.Second})
	require.Equal(t, BreakerClosed, b.State())
	assert.True(t, b.Allow(), "Closed should allow calls")
}

// TestBreaker_OpenAfterConsecutiveFailures 连续 N 次失败 → Open。
func TestBreaker_OpenAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 3, OpenSeconds: 30 * time.Second})
	b.RecordFailure()
	b.RecordFailure()
	assert.Equal(t, BreakerClosed, b.State(), "still closed after 2 fails")
	b.RecordFailure()
	assert.Equal(t, BreakerOpen, b.State(), "opens after 3rd consecutive fail")
}

// TestBreaker_OpenRejectsCalls Open 状态拒绝调用。
func TestBreaker_OpenRejectsCalls(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: 30 * time.Second})
	b.RecordFailure() // → Open
	require.Equal(t, BreakerOpen, b.State())
	assert.False(t, b.Allow(), "Open should reject calls")
}

// TestBreaker_HalfOpenAfterOpenTimeout Open 持续 OpenSeconds 后转 HalfOpen。
func TestBreaker_HalfOpenAfterOpenTimeout(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: 50 * time.Millisecond})
	b.RecordFailure()
	require.Equal(t, BreakerOpen, b.State())
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, BreakerHalfOpen, b.State(), "should transition to HalfOpen after OpenSeconds")
}

// TestBreaker_HalfOpenAllowsOneProbe HalfOpen 允许 1 次试探。
func TestBreaker_HalfOpenAllowsOneProbe(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: 30 * time.Second})
	b.RecordFailure()
	b.nowFunc = func() time.Time { return time.Now().Add(31 * time.Second) } // 模拟时间过去
	require.Equal(t, BreakerHalfOpen, b.State())
	assert.True(t, b.Allow(), "HalfOpen allows one probe")
}

// TestBreaker_HalfOpenSuccessClosesBreaker HalfOpen 试探成功 → Closed。
func TestBreaker_HalfOpenSuccessClosesBreaker(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: 30 * time.Second})
	b.RecordFailure()
	b.nowFunc = func() time.Time { return time.Now().Add(31 * time.Second) }
	require.Equal(t, BreakerHalfOpen, b.State())
	b.RecordSuccess()
	assert.Equal(t, BreakerClosed, b.State())
}

// TestBreaker_HalfOpenFailureReopens HalfOpen 试探失败 → Open。
func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: 30 * time.Second})
	b.RecordFailure()
	b.nowFunc = func() time.Time { return time.Now().Add(31 * time.Second) }
	require.Equal(t, BreakerHalfOpen, b.State())
	b.RecordFailure()
	assert.Equal(t, BreakerOpen, b.State())
}

// TestBreaker_ClosedSuccessResetsFailureCount Closed 状态下成功调用重置失败计数。
func TestBreaker_ClosedSuccessResetsFailureCount(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 3, OpenSeconds: 30 * time.Second})
	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess() // 重置
	b.RecordFailure()
	b.RecordFailure()
	assert.Equal(t, BreakerClosed, b.State(), "consecutive count should reset on success")
	b.RecordFailure()
	assert.Equal(t, BreakerOpen, b.State())
}

// TestBreaker_RecordResultHelper 便捷 helper 同时记录 success/fail。
func TestBreaker_RecordResultHelper(t *testing.T) {
	t.Parallel()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 2, OpenSeconds: 30 * time.Second})
	b.RecordResult(nil)
	b.RecordResult(errors.New("boom"))
	b.RecordResult(errors.New("boom"))
	assert.Equal(t, BreakerOpen, b.State())
}