package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHotReloadLimiter_Default：当 Update 未触发时返回构造默认值。
func TestHotReloadLimiter_Default(t *testing.T) {
	l := NewHotReloadLimiter(60, 100)
	limit, burst := l.Snapshot()
	assert.Equal(t, 60, limit)
	assert.Equal(t, 100, burst)
}

// TestHotReloadLimiter_UpdateAppliesNewValues：Update 后 Snapshot 返回新值。
func TestHotReloadLimiter_UpdateAppliesNewValues(t *testing.T) {
	l := NewHotReloadLimiter(60, 100)
	l.Update(OpsConfig{LimitCount: 30, Burst: 50})
	limit, burst := l.Snapshot()
	assert.Equal(t, 30, limit)
	assert.Equal(t, 50, burst)
}

// TestHotReloadLimiter_UpdateIgnoresZero：0 值不覆盖（保留上一有效值）。
func TestHotReloadLimiter_UpdateIgnoresZero(t *testing.T) {
	l := NewHotReloadLimiter(60, 100)
	l.Update(OpsConfig{LimitCount: 30})
	l.Update(OpsConfig{Burst: 50})
	limit, burst := l.Snapshot()
	assert.Equal(t, 30, limit)
	assert.Equal(t, 50, burst)
}

// TestHotReloadLimiter_ConcurrentSafe：并发 Update + Snapshot 不 race。
func TestHotReloadLimiter_ConcurrentSafe(t *testing.T) {
	l := NewHotReloadLimiter(60, 100)
	done := make(chan struct{})
	go func() {
		for i := 1; i <= 100; i++ {
			l.Update(OpsConfig{LimitCount: i})
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_, _ = l.Snapshot()
	}
	<-done
	limit, _ := l.Snapshot()
	require.Greater(t, limit, 0)
}