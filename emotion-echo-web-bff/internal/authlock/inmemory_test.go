package authlock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestInMemoryStore_LockAfterMaxFailures Round 4.3 后补：登录失败
// 累计到 maxFailures 后应触发锁定。
func TestInMemoryStore_LockAfterMaxFailures(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	// 前 4 次失败 → 未锁
	for i := 0; i < loginMaxFailures-1; i++ {
		locked := s.RecordFailure(ctx, "alice")
		assert.False(t, locked, "第 %d 次失败应未触发锁定", i+1)
		assert.False(t, s.IsLocked(ctx, "alice"), "第 %d 次后仍应未锁", i+1)
	}

	// 第 5 次失败 → 触发锁定
	locked := s.RecordFailure(ctx, "alice")
	assert.True(t, locked, "第 5 次失败应触发锁定")
	assert.True(t, s.IsLocked(ctx, "alice"), "应被锁")
}

// TestInMemoryStore_LockWindowExpires Round 4.3 后补：锁定窗口
// 过期后 IsLocked 返 false（不主动清，等下次 RecordFailure 累加）。
func TestInMemoryStore_LockWindowExpires(t *testing.T) {
	s := NewInMemoryStore()
	s.lockWindow = 50 * time.Millisecond // 测试用短窗口
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	assert.True(t, s.IsLocked(ctx, "alice"))

	time.Sleep(60 * time.Millisecond)
	assert.False(t, s.IsLocked(ctx, "alice"), "锁定窗口过期后应未锁")
}

// TestInMemoryStore_ClearFailures Round 4.3 后补：ClearFailures 清除后
// IsLocked 返 false，count 可继续累加。
func TestInMemoryStore_ClearFailures(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	s.RecordFailure(ctx, "alice")
	s.RecordFailure(ctx, "alice")
	s.ClearFailures(ctx, "alice")

	// 重置后重新累加到 maxFailures 才锁定
	for i := 0; i < loginMaxFailures-1; i++ {
		assert.False(t, s.RecordFailure(ctx, "alice"))
	}
	assert.True(t, s.RecordFailure(ctx, "alice"))
	assert.True(t, s.IsLocked(ctx, "alice"))
}

// TestInMemoryStore_LockedDoesNotIncrement Round 4.3 后补：锁定期内
// 重复失败不增加计数（防恶意打爆）。
func TestInMemoryStore_LockedDoesNotIncrement(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	assert.True(t, s.IsLocked(ctx, "alice"))

	// 锁定期内再次失败
	for i := 0; i < 10; i++ {
		locked := s.RecordFailure(ctx, "alice")
		assert.False(t, locked, "锁定期内不应触发新锁定")
	}
	assert.True(t, s.IsLocked(ctx, "alice"), "仍处于锁定状态")
}

// TestInMemoryStore_PerUsernameIsolation Round 4.3 后补：不同 username
// 独立计数。
func TestInMemoryStore_PerUsernameIsolation(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	for i := 0; i < loginMaxFailures; i++ {
		s.RecordFailure(ctx, "alice")
	}
	assert.True(t, s.IsLocked(ctx, "alice"))
	assert.False(t, s.IsLocked(ctx, "bob"), "bob 不受 alice 锁定影响")
}

// TestInMemoryStore_VerificationCode_MinGap Round 4.3 后补：验证码
// 60s 内不可重复发。
func TestInMemoryStore_VerificationCode_MinGap(t *testing.T) {
	s := NewInMemoryStore()
	s.minGap = 50 * time.Millisecond // 测试用短间隔
	ctx := context.Background()

	assert.True(t, s.CanSendVerificationCode(ctx, "alice"), "首次应可发")

	s.SaveVerificationCode(ctx, "alice", "123456", 5*time.Minute)
	assert.False(t, s.CanSendVerificationCode(ctx, "alice"), "minGap 内不可重发")

	time.Sleep(60 * time.Millisecond)
	assert.True(t, s.CanSendVerificationCode(ctx, "alice"), "minGap 过后可重发")
}

// TestInMemoryStore_VerificationCode_RoundTrip Round 4.3 后补：
// Save + Get 端到端 + TTL 过期。
func TestInMemoryStore_VerificationCode_RoundTrip(t *testing.T) {
	s := NewInMemoryStore()
	ctx := context.Background()

	err := s.SaveVerificationCode(ctx, "alice", "654321", 50*time.Millisecond)
	assert.NoError(t, err)

	code, err := s.GetVerificationCode(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, "654321", code)

	time.Sleep(60 * time.Millisecond)
	code, err = s.GetVerificationCode(ctx, "alice")
	assert.NoError(t, err)
	assert.Equal(t, "", code, "TTL 过期应返空")
}

// TestInMemoryStore_InterfaceConformance Round 4.3 后补：接口契约。
func TestInMemoryStore_InterfaceConformance(t *testing.T) {
	var s LoginLockStore = NewInMemoryStore()
	ctx := context.Background()

	s.RecordFailure(ctx, "u1")
	assert.False(t, s.IsLocked(ctx, "u1"))
	s.ClearFailures(ctx, "u1")
	assert.NoError(t, s.ClearFailures(ctx, "u1"))
}

// vcStoreContract E2E-20 收尾裁定（D-01，2026-09-28 用户确认）：
// 验证码存储与登录锁定存储必须是两个独立接口——验证码端点是 D-01 决议
// 下的遗留物（E2E-F-144 待删），只允许 in-memory，禁止 Redis 化。
// 这里用局部接口声明做契约断言，不依赖生产代码是否已拆出该接口。
type vcStoreContract interface {
	CanSendVerificationCode(ctx context.Context, username string) bool
	SaveVerificationCode(ctx context.Context, username, code string, ttl time.Duration) error
	GetVerificationCode(ctx context.Context, username string) (string, error)
}

// TestInterfaceShrink_VerificationCodeStore_NotRedis E2E-20 收尾：
//   - InMemoryStore 必须实现验证码存储契约（遗留端点的唯一合法后端）
//   - RedisStore 不得实现验证码存储契约（裁定：验证码禁止跨实例共享存储）
func TestInterfaceShrink_VerificationCodeStore_NotRedis(t *testing.T) {
	assert.Implements(t, (*vcStoreContract)(nil), NewInMemoryStore(),
		"InMemoryStore 应实现验证码存储契约")
	assert.NotImplements(t, (*vcStoreContract)(nil), NewRedisStore(RedisConfig{}),
		"RedisStore 不得实现验证码存储契约（D-01 裁定：验证码为遗留端点，禁止 Redis 化）")
}