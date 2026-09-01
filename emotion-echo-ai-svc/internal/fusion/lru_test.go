// Package fusion — Stage 35 · PR-3 RED
//
// msgIDLRU 是 FusionWorker 用的同 messageID 限流器。
//
// 设计：
//   - 纯内存 LRU（container/list + map[int64]*list.Element）
//   - cap 默认 1024（可通过 NewMsgIDLRU 覆盖）
//   - TTL：若 msgID 在 TTL 内被 Add 过，Touch 返回 true（应跳过）
//   - 线程安全：sync.Mutex
//
// 行为约定：
//   - Add(msgID)：记录当前时间戳，若 cap 满则 Evict 最久未用
//   - Touch(msgID)：若 msgID 存在且未过期 → 返回 true（命中）；否则返回 false
//   - 不存在/过期 → Add + 返回 false
package fusion

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMsgIDLRU_NewReturnsNonNil 构造器返回非 nil。
func TestMsgIDLRU_NewReturnsNonNil(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(100, time.Minute)
	require.NotNil(t, lru)
	assert.Equal(t, 0, lru.Len())
}

// TestMsgIDLRU_TouchMissFirstTime 首次 Touch → false（未命中）+ 内部记录。
func TestMsgIDLRU_TouchMissFirstTime(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(100, time.Minute)
	assert.False(t, lru.Touch(1), "first Touch should miss")
	assert.Equal(t, 1, lru.Len())
}

// TestMsgIDLRU_TouchHitAfterAdd Add 后 Touch → true（命中）。
func TestMsgIDLRU_TouchHitAfterAdd(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(100, time.Minute)
	lru.Add(1)
	assert.True(t, lru.Touch(1), "Touch after Add should hit")
}

// TestMsgIDLRU_TouchMissAfterTTLExpired 超过 TTL → false。
func TestMsgIDLRU_TouchMissAfterTTLExpired(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(100, 50*time.Millisecond)
	lru.Add(1)
	time.Sleep(100 * time.Millisecond)
	assert.False(t, lru.Touch(1), "Touch after TTL should miss")
}

// TestMsgIDLRU_EvictionAtCapacity 容量满 → 驱逐最久未用。
func TestMsgIDLRU_EvictionAtCapacity(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(3, time.Minute)
	lru.Add(1)
	lru.Add(2)
	lru.Add(3)
	// 已满
	lru.Add(4) // 触发驱逐 1
	assert.Equal(t, 3, lru.Len())
	assert.False(t, lru.Touch(1), "msgID=1 should be evicted")
	assert.True(t, lru.Touch(2))
	assert.True(t, lru.Touch(3))
	assert.True(t, lru.Touch(4))
}

// TestMsgIDLRU_TouchUpdatesRecency Touch 命中会更新 recency，影响后续驱逐顺序。
func TestMsgIDLRU_TouchUpdatesRecency(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(3, time.Minute)
	lru.Add(1)
	lru.Add(2)
	lru.Add(3)
	// 触摸 1，让它变"最新"
	assert.True(t, lru.Touch(1))
	// 加 4，应驱逐 2（最久未触摸）
	lru.Add(4)
	assert.False(t, lru.Touch(2), "msgID=2 should be evicted")
	assert.True(t, lru.Touch(1), "msgID=1 still alive due to recent Touch")
	assert.True(t, lru.Touch(3))
	assert.True(t, lru.Touch(4))
}

// TestMsgIDLRU_ConcurrentSafe 并发 Add/Touch 不应 race（go test -race）。
func TestMsgIDLRU_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	lru := NewMsgIDLRU(100, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				lru.Add(id)
				_ = lru.Touch(id)
			}
		}(int64(i))
	}
	wg.Wait()
	assert.LessOrEqual(t, lru.Len(), 100, "should not exceed cap")
}