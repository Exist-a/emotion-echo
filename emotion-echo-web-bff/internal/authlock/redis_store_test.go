package authlock

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisStore Round 4.3 后补：测试用 miniredis + RedisStore 装配
func newTestRedisStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewRedisStore(RedisConfig{Client: client, Prefix: "test"}), mr
}

// TestRedisStore_LockAfterMaxFailures Round 4.3 后补：失败
// 累计到 maxFailures 后触发锁定。
func TestRedisStore_LockAfterMaxFailures(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	for i := 0; i < loginMaxFailures-1; i++ {
		locked := s.RecordFailure(ctx, "alice")
		assert.False(t, locked, "第 %d 次失败应未锁", i+1)
		assert.False(t, s.IsLocked(ctx, "alice"), "第 %d 次后仍应未锁", i+1)
	}
	locked := s.RecordFailure(ctx, "alice")
	assert.True(t, locked, "第 5 次应触锁")
	assert.True(t, s.IsLocked(ctx, "alice"), "应被锁")
}

// TestRedisStore_LockWindowExpires Round 4.3 后补：锁定窗口过期。
func TestRedisStore_LockWindowExpires(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	s := NewRedisStore(RedisConfig{
		Client:  client,
		Prefix:  "test",
		LockTTL: 100 * time.Millisecond, // 短窗口用于测试
	})

	ctx := context.Background()
	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	assert.True(t, s.IsLocked(ctx, "alice"))

	mr.FastForward(200 * time.Millisecond)
	assert.False(t, s.IsLocked(ctx, "alice"), "锁定窗口过期后应未锁")
}

// TestRedisStore_ClearFailures Round 4.3 后补：ClearFailures 清除后
// 可重新触发。
func TestRedisStore_ClearFailures(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	s.RecordFailure(ctx, "alice")
	s.RecordFailure(ctx, "alice")
	require.NoError(t, s.ClearFailures(ctx, "alice"))

	// 重新累加到 maxFailures 才锁
	for i := 0; i < loginMaxFailures-1; i++ {
		assert.False(t, s.RecordFailure(ctx, "alice"))
	}
	assert.True(t, s.RecordFailure(ctx, "alice"))
}

// TestRedisStore_LockedDoesNotIncrement Round 4.3 后补：锁定期内重复
// 失败不增加计数。
func TestRedisStore_LockedDoesNotIncrement(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	require.True(t, s.IsLocked(ctx, "alice"))

	for i := 0; i < 10; i++ {
		locked := s.RecordFailure(ctx, "alice")
		assert.False(t, locked, "锁定期内不应触发新锁定")
	}
	assert.True(t, s.IsLocked(ctx, "alice"), "仍处于锁定状态")
}

// TestRedisStore_PerUsernameIsolation Round 4.3 后补：不同 username 独立。
func TestRedisStore_PerUsernameIsolation(t *testing.T) {
	s, _ := newTestRedisStore(t)
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	assert.True(t, s.IsLocked(ctx, "alice"))
	assert.False(t, s.IsLocked(ctx, "bob"))
}

// TestRedisStore_CrossInstanceConsistency Round 4.3 后补：E2E-20
// 核心 RED 修复验证——**两个 RedisStore 实例**（模拟两个 BFF）共享
// 同一 Redis 后端时，跨实例计数一致。
func TestRedisStore_CrossInstanceConsistency(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// 两个实例 = 两个 RedisStore，共享同一 Redis 客户端 = 共享数据
	bff1 := NewRedisStore(RedisConfig{Client: client, Prefix: "ci"})
	bff2 := NewRedisStore(RedisConfig{Client: client, Prefix: "ci"})

	ctx := context.Background()

	// BFF-1 触发锁定
	for i := 0; i < loginMaxFailures; i++ {
		bff1.RecordFailure(ctx, "alice")
	}
	require.True(t, bff1.IsLocked(ctx, "alice"))

	// BFF-2 应**自动看到**锁定（共享 Redis）
	assert.True(t, bff2.IsLocked(ctx, "alice"), "BFF-2 应通过 Redis 共享状态看到 BFF-1 触发的锁定")

	// BFF-2 清除（登录成功）→ BFF-1 应也看不到
	require.NoError(t, bff2.ClearFailures(ctx, "alice"))
	assert.False(t, bff1.IsLocked(ctx, "alice"), "BFF-2 清除后 BFF-1 也应未锁")
}

// TestRedisStore_VerificationCode_* 用例已随 E2E-20 收尾裁定移除：
// 验证码存储从 RedisStore 拆出（D-01 裁定遗留端点禁止 Redis 化，
// 见 store.go VerificationCodeStore），in-memory 用例保留在 inmemory_test.go。

// TestRedisStore_RedisDown_DegradeAllow Round 4.3 后补：Redis 不可达
// 时降级 = 返 false（不 fail-closed）。这是 E2E-20 #5 [M] 项决议——
// 降级不更糟（不拒绝登录）。
//
// 注：与 LimiterBackend 不同，LoginLockStore 降级到 in-memory 由调用方
// 决定（main.go 装配）。本测试只验证 RedisStore 本身的降级行为：
// Redis 不可达 → IsLocked 返 false（不锁）；RecordFailure 返 false
// （不计数累加）。
func TestRedisStore_RedisDown_DegradeAllow(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer client.Close()

	s := NewRedisStore(RedisConfig{Client: client, Prefix: "degrade", Timeout: 30 * time.Millisecond})
	ctx := context.Background()

	// Redis 不可达 → IsLocked 返 false（不更糟）
	assert.False(t, s.IsLocked(ctx, "alice"))
	// RecordFailure 返 false（不触发锁定）
	assert.False(t, s.RecordFailure(ctx, "alice"))
}

// TestRedisStore_InterfaceConformance Round 4.3 后补：接口契约。
func TestRedisStore_InterfaceConformance(t *testing.T) {
	s, _ := newTestRedisStore(t)
	var iface LoginLockStore = s
	ctx := context.Background()

	iface.RecordFailure(ctx, "u1")
	iface.IsLocked(ctx, "u1")
	iface.ClearFailures(ctx, "u1")
}