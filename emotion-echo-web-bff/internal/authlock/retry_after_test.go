package authlock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-29 遗留项 1（2026-10-08）：423 锁定响应补 `Retry-After`。
//
// 测试点 #14 的通过标准要求"`Retry-After` 存在且与前端指数退避策略一致"，
// 而收口时 423 无该头（仅靠前端兜底）。为使 handler 能给出**剩余**锁定秒数
// （而非恒等于满窗口），store 暴露 RetryAfter。

// TestInMemoryStore_RetryAfter_ZeroWhenNotLocked：未锁定 → 0（handler 据此不发头）。
func TestInMemoryStore_RetryAfter_ZeroWhenNotLocked(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	assert.Zero(t, s.RetryAfter(ctx, "nobody"), "未知用户应返 0")

	// 未达阈值（4 次）→ 仍未锁 → 0
	for i := 0; i < loginMaxFailures-1; i++ {
		s.RecordFailure(ctx, "alice")
	}
	assert.Zero(t, s.RetryAfter(ctx, "alice"), "未达阈值不应返剩余时间")
}

// TestInMemoryStore_RetryAfter_PositiveWhenLocked：锁定后返剩余时间，且 ≤ 锁定窗口。
func TestInMemoryStore_RetryAfter_PositiveWhenLocked(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	require.True(t, s.IsLocked(ctx, "alice"), "前置：应已锁定")

	got := s.RetryAfter(ctx, "alice")
	assert.Greater(t, got, time.Duration(0), "锁定后 RetryAfter 必须 > 0")
	assert.LessOrEqual(t, got, loginLockWindow, "不得超过锁定窗口")
	assert.Greater(t, got, loginLockWindow-time.Minute, "刚锁定应接近满窗口（剩余 = 窗口 - 已过）")
}

// TestInMemoryStore_RetryAfter_ZeroAfterWindowExpires：窗口过期后 → 0（与 IsLocked 一致）。
func TestInMemoryStore_RetryAfter_ZeroAfterWindowExpires(t *testing.T) {
	s := NewInMemoryStore()
	s.lockWindow = 50 * time.Millisecond // 测试用短窗口
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	require.True(t, s.IsLocked(ctx, "alice"), "前置：应已锁定")

	time.Sleep(80 * time.Millisecond)

	assert.False(t, s.IsLocked(ctx, "alice"), "前置：窗口应已过期")
	assert.Zero(t, s.RetryAfter(ctx, "alice"), "窗口过期后应返 0")
}

// TestRedisStore_RetryAfter_ZeroWhenNotLocked / PositiveWhenLocked：
// Redis 实现与 in-memory 行为一致。
func TestRedisStore_RetryAfter(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	assert.Zero(t, s.RetryAfter(ctx, "alice"), "未锁定应返 0")

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	require.True(t, s.IsLocked(ctx, "alice"), "前置：应已锁定")

	got := s.RetryAfter(ctx, "alice")
	assert.Greater(t, got, time.Duration(0), "锁定后 RetryAfter 必须 > 0")
	assert.LessOrEqual(t, got, loginLockWindow, "不得超过锁定窗口")
}
