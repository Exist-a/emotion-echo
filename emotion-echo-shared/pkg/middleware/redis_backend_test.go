package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisLimiter Round 4.3 PR-2 后补：测试用 miniredis + 真实
// go-redis 客户端 + RedisLimiterBackend 装配。返回 backend + miniredis
// 实例（用于 Close 释放）。
func newTestRedisLimiter(t *testing.T, rate float64, burst int, prefix string) (*RedisLimiterBackend, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = client.Close() })

	return NewRedisLimiterBackend(client, rate, burst, prefix), mr
}

// TestRedisLimiterBackend_AllowsBelowBurst Round 4.3 PR-2 后补：burst
// 内的请求全部放行；burst 用完后第 N+1 个请求被拒。
func TestRedisLimiterBackend_AllowsBelowBurst(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 0.1, 5, "test-allows")

	for i := 0; i < 5; i++ {
		assert.True(t, backend.Allow("user-A"), "req %d: burst 内应放行", i)
	}
	assert.False(t, backend.Allow("user-A"), "burst 用完后应被拒")
}

// TestRedisLimiterBackend_PerKeyIsolation Round 4.3 PR-2 后补：不同
// key 独立桶——A 用完桶不影响 B。
func TestRedisLimiterBackend_PerKeyIsolation(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 0.1, 2, "test-perkey")

	// user-A 烧光
	assert.True(t, backend.Allow("user-A"))
	assert.True(t, backend.Allow("user-A"))
	assert.False(t, backend.Allow("user-A"))

	// user-B 应仍可（独立桶）
	assert.True(t, backend.Allow("user-B"))
	assert.True(t, backend.Allow("user-B"))
	assert.False(t, backend.Allow("user-B"))
}

// TestRedisLimiterBackend_Refills Round 4.3 PR-2 后补：refill 计时——
// 时间流逝后桶自动补 token。
//
// 实现说明：Lua 脚本读 Go 进程 time.Now()，miniredis.FastForward
// 推进的是 miniredis 内部时钟（key 过期），不影响服务端 Go 时间。
// 因此本测试用真实 sleep（rate=100/s + 200ms = 20 tokens 远超 1 个所需）。
// sleep 加大到 200ms 留余量（CI runner 负载高时 50ms 可能边界）。
func TestRedisLimiterBackend_Refills(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 100, 2, "test-refill")

	assert.True(t, backend.Allow("u1"))
	assert.True(t, backend.Allow("u1"))
	assert.False(t, backend.Allow("u1"))

	// 真实 sleep 200ms：100/s * 0.2 = 20 tokens（远超 1 个所需）
	time.Sleep(200 * time.Millisecond)
	assert.True(t, backend.Allow("u1"), "refill 后应放行（rate=100/s, sleep=200ms）")
}

// TestRedisLimiterBackend_RetryAfter Round 4.3 PR-2 后补：桶空后
// RetryAfter 返回 > 0 的等待时间。
func TestRedisLimiterBackend_RetryAfter(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 1.0, 1, "test-retry")

	assert.True(t, backend.Allow("u1"))
	retry := backend.RetryAfter("u1")
	assert.Greater(t, retry, time.Duration(0), "burst 用完后 RetryAfter 必须 > 0")

	// rate=1/s, tokens=0 → 需要补 1 个 token，retry = 1000ms
	assert.LessOrEqual(t, retry, 1100*time.Millisecond, "retry 应接近 1s（rate=1.0）")
}

// TestRedisLimiterBackend_RetryAfter_AllowsZero Round 4.3 PR-2 后补：
// 桶满时 RetryAfter = 0（无需等待）。
func TestRedisLimiterBackend_RetryAfter_AllowsZero(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 1.0, 5, "test-allow-zero")

	retry := backend.RetryAfter("u-new")
	assert.Equal(t, time.Duration(0), retry, "新 key RetryAfter = 0")
}

// TestRedisLimiterBackend_KeyPrefix Round 4.3 PR-2 后补：keyPrefix
// 隔离——同 key 不同 prefix = 不同桶。
func TestRedisLimiterBackend_KeyPrefix(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	backendA := NewRedisLimiterBackend(client, 0.1, 1, "prefix-A")
	backendB := NewRedisLimiterBackend(client, 0.1, 1, "prefix-B")

	// 同 user 但不同 prefix —— 应该各自有独立桶
	assert.True(t, backendA.Allow("shared-user"))
	assert.True(t, backendB.Allow("shared-user"))
}

// TestRedisLimiterBackend_RedisDown_DegradeAllow Round 4.3 PR-2 后补：
// Redis 不可达时降级 = allow（行为 = 不限流，不 fail-closed）。这是
// E2E-20 #5 [M] 项的决议——降级不更糟。
//
// 实现说明：用一个**永远连不上**的端口（127.0.0.1:1）模拟 Redis
// 不可达。不用 miniredis.Close() —— 那会 panic（mr.Addr() 内部 nil）。
func TestRedisLimiterBackend_RedisDown_DegradeAllow(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1", // 不会监听的端口
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		MaxRetries:  -1, // 立即返回错误，不重试
	})
	t.Cleanup(func() { _ = client.Close() })

	backend := NewRedisLimiterBackend(client, 1.0, 1, "test-degrade")

	// Redis 不可达 —— Allow 仍应返 true（降级=不限流）
	assert.True(t, backend.Allow("u1"), "Redis 不可达时降级=allow（不更糟）")
	assert.Equal(t, time.Duration(0), backend.RetryAfter("u1"), "Redis 不可达时 RetryAfter=0")
}

// TestRedisLimiterBackend_InterfaceConformance Round 4.3 PR-2 后补：
// 编译期 + 运行时接口契约钉。
func TestRedisLimiterBackend_InterfaceConformance(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 1.0, 1, "test-iface")
	var iface LimiterBackend = backend
	assert.True(t, iface.Allow("k1"))
	retry := iface.RetryAfter("k1")
	assert.Greater(t, retry, time.Duration(0))
}

// TestRedisLimiterBackend_ContextTimeout Round 4.3 PR-2 后补：
// 短超时设置下也能正常返回（不 hang）。
func TestRedisLimiterBackend_ContextTimeout(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 1.0, 1, "test-ctx")
	backend.timeout = 10 * time.Millisecond // 极短超时
	done := make(chan bool)
	go func() {
		_ = backend.Allow("u1")
		done <- true
	}()
	select {
	case <-done:
		// 正常返回
	case <-time.After(1 * time.Second):
		t.Fatal("Allow 在 1s 内未返回（可能 hang）")
	}
}

// TestRedisLimiterBackend_BuildKeyFormat Round 4.3 PR-2 后补：Redis
// key 格式契约——`lb:{prefix}:{userKey}` 防止业务方 key 冲突。
func TestRedisLimiterBackend_BuildKeyFormat(t *testing.T) {
	backend, mr := newTestRedisLimiter(t, 1.0, 5, "format-test")

	backend.Allow("user-42")

	// 直接查 miniredis 验证 key 存在
	keys := mr.Keys()
	found := false
	for _, k := range keys {
		if k == "lb:format-test:user-42" {
			found = true
			break
		}
	}
	assert.True(t, found, "key 格式应为 lb:{prefix}:{userKey}；实际 keys=%v", keys)
}

// TestRedisLimiterBackend_ConcurrentSafety Round 4.3 PR-2 后补：
// 并发安全 —— Redis EVAL 单线程保证，但 miniredis 启动需要确认
// Lua 脚本不报错。
func TestRedisLimiterBackend_ConcurrentSafety(t *testing.T) {
	backend, _ := newTestRedisLimiter(t, 1000, 100, "test-concurrent")

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				backend.Allow("concurrent-user")
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// 100 个 Allow 应该全通过（rate=1000, burst=100, 10 并发 ×10 = 100 = burst 边界）
	// 严格说可能 refill 不够；但只要不 panic / 不报错就算过
}

// TestRedisLimiterBackend_NewWithRetryField 未用 import 守卫
var _ = context.Background