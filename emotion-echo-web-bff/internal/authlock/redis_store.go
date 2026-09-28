// Round 4.3 后补：LoginLockStore 的 Redis 实现。
//
// 多 BFF 实例下共享登录失败计数 + 验证码缓存。解决 E2E-20 测试点 #2
// （登录锁定跨实例失效）和 #3（验证码防枚举跨实例失效）。
//
// Key 设计：
//   - 失败计数：lock:fails:{username}（String，存 failCount:lockedAtUnix）
//   - 验证码：vercode:{username}（String，存 code + expiresAt）
//
// 降级（E2E-20 #5 [M]）：Redis 不可达时返 ErrStoreDown，由调用方
// 决定 fallback（fallback 到 in-memory 实现）。
package authlock

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore Round 4.3 后补：Redis LoginLockStore 实现
type RedisStore struct {
	client    *redis.Client
	prefix    string        // key 前缀（多业务方隔离）
	timeout   time.Duration // 单次操作超时
	lockTTL   time.Duration // 失败计数的 Redis TTL（默认 = lockWindow + 1min）
	codeTTL   time.Duration // 验证码的 Redis TTL（默认 = verificationTTL）
	minGap    time.Duration // 验证码最小间隔（默认 = verificationMinGap）
}

// RedisConfig 配置选项
type RedisConfig struct {
	Client  *redis.Client
	Prefix  string        // 默认 "authlock"
	Timeout time.Duration // 默认 100ms
	LockTTL time.Duration // 默认 = lockWindow + 1min
	CodeTTL time.Duration // 默认 = verificationTTL
	MinGap  time.Duration // 默认 = verificationMinGap（60s）
}

// NewRedisStore 构造 RedisStore
func NewRedisStore(cfg RedisConfig) *RedisStore {
	if cfg.Prefix == "" {
		cfg.Prefix = "authlock"
	}
	if cfg.Timeout == 0 {
		// E2E-20 GREEN 修复：100ms 对首次 TCP 拨号 + DNS + EVAL 太短 ⇒ 静默降级
		// （Redis keys 恒空、RecordFailure 恒返 false 的真根因）。1s 覆盖冷连接；
		// 热连接（池复用）实际 <5ms。
		cfg.Timeout = 1 * time.Second
	}
	if cfg.LockTTL == 0 {
		cfg.LockTTL = loginLockWindow + time.Minute
	}
	if cfg.CodeTTL == 0 {
		cfg.CodeTTL = verificationTTL
	}
	if cfg.MinGap == 0 {
		cfg.MinGap = verificationMinGap
	}
	return &RedisStore{
		client:  cfg.Client,
		prefix:  cfg.Prefix,
		timeout: cfg.Timeout,
		lockTTL: cfg.LockTTL,
		codeTTL: cfg.CodeTTL,
		minGap:  cfg.MinGap,
	}
}

// Redis 内部 key 构造
func (s *RedisStore) failKey(username string) string {
	return fmt.Sprintf("%s:fails:%s", s.prefix, username)
}

func (s *RedisStore) codeKey(username string) string {
	return fmt.Sprintf("%s:vercode:%s", s.prefix, username)
}

// parseFailCount Round 4.3 后补：读 failKey 的 hash field 'fails'
func parseFailCount(raw string) int {
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

// parseLockedAtMs Round 4.3 后补：读 failKey 的 hash field 'locked_at'
func parseLockedAtMs(raw string) int64 {
	if raw == "" {
		return 0
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	return n
}

// IsLocked Round 4.3 后补：检查 username 是否被锁
//
// 实现：从 Redis HGET failKey 的 locked_at field；
// 若 > 0 且 (now - locked_at) < lockTTL → 锁
// Redis 不可达时返 false（降级，不 fail-closed）
func (s *RedisStore) IsLocked(ctx context.Context, username string) bool {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	val, err := s.client.HGet(c, s.failKey(username), "locked_at").Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false
		}
		return false // Redis 不可达 → 降级
	}
	lockedAtMs := parseLockedAtMs(val)
	if lockedAtMs == 0 {
		return false
	}
	// lockTTL = lockWindow + 1min（grace 让 Redis TTL 略长于实际锁定窗口）
	// 但 IsLocked 应该按 lockWindow 算（与 in-memory 一致）
	return time.Now().UnixMilli()-lockedAtMs < loginLockWindow.Milliseconds()
}

// RecordFailure Round 4.3 后补：记录失败
//
// 实现：Redis Lua 脚本原子读 + 写：
//   - 若 lockedAt+lockWindow > now（仍锁中）→ 不修改
//   - 否则 failCount++，若 failCount ≥ maxFailures → lockedAt=now, failCount=0
// 返 true 表示这次触发锁定
func (s *RedisStore) RecordFailure(ctx context.Context, username string) bool {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	now := time.Now().UnixMilli()

	// Lua 脚本：原子读 + 判断 + 写
	res, err := redisRecordFailureScript.Run(c, s.client,
		[]string{s.failKey(username)},
		now, s.lockTTL.Milliseconds(), loginMaxFailures).Result()
	if err != nil {
		// E2E-20 GREEN 诊断：静默降级掩盖真错误（100ms→1s 修复后 keys 仍空）。
		// 打日志让运维能定位（降级行为不变——不 fail-closed）。
		log.Printf("[authlock] RecordFailure redis err (degraded): %v", err)
		return false // Redis 不可达 → 降级不更糟
	}
	arr, ok := res.([]any)
	if !ok || len(arr) < 2 {
		return false
	}
	triggered, _ := arr[0].(int64)
	return triggered == 1
}

// ClearFailures Round 4.3 后补：清除失败计数
func (s *RedisStore) ClearFailures(ctx context.Context, username string) error {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.client.Del(c, s.failKey(username)).Err()
}

// CanSendVerificationCode Round 4.3 后补：检查验证码 minGap 间隔
//
// 与 in-memory 一致：距上次发送不到 minGap → false（不可重发）
// Redis 不可达 → true（降级允许）
func (s *RedisStore) CanSendVerificationCode(ctx context.Context, username string) bool {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	val, err := s.client.Get(c, s.codeKey(username)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return true // 无码 = 可发
		}
		return true // 降级 = 允许
	}
	parts := splitVal(val)
	if len(parts) != 2 {
		return true
	}
	lastSentMs, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return true
	}
	return time.Now().UnixMilli()-lastSentMs >= s.minGap.Milliseconds()
}

// SaveVerificationCode Round 4.3 后补：保存验证码 + TTL
func (s *RedisStore) SaveVerificationCode(ctx context.Context, username, code string, ttl time.Duration) error {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	now := time.Now().UnixMilli()
	val := code + "|" + strconv.FormatInt(now, 10)
	// 使用传入的 ttl 而非 s.codeTTL（让调用方控制）
	return s.client.Set(c, s.codeKey(username), val, ttl).Err()
}

// GetVerificationCode Round 4.3 后补：取验证码（不存在或过期返 ("", nil)）
func (s *RedisStore) GetVerificationCode(ctx context.Context, username string) (string, error) {
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	val, err := s.client.Get(c, s.codeKey(username)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	parts := splitVal(val)
	if len(parts) != 2 {
		return "", nil
	}
	return parts[0], nil
}

// Close Round 4.3 后补：关闭 Redis 连接
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// Redis 原子操作 Lua 脚本（RecordFailure）
//
// KEYS[1]: failKey（Hash with 'fails' + 'locked_at' fields）
// ARGV[1]: nowMs
// ARGV[2]: lockTTLMs（用于 EXPIRE）
// ARGV[3]: maxFailures
//
// 返回：
//   {triggered: 1 or 0, failCount: int}
//
// 逻辑：
//   - 读 failKey 的 fails + locked_at fields（空=首次）
//   - 若 locked_at > 0 且 (nowMs - locked_at) < lockTTLMs → 仍锁中
//     → 不修改，返 {0, failCount}
//   - 否则 failCount++
//   - 若 failCount ≥ maxFailures → locked_at=nowMs, failCount=0, triggered=1
//   - 否则 failCount 不变, triggered=0
//   - HSET 写入 + PEXPIRE
var redisRecordFailureScript = redis.NewScript(`
local fails = tonumber(redis.call('HGET', KEYS[1], 'fails'))
local locked_at = tonumber(redis.call('HGET', KEYS[1], 'locked_at'))
if fails == nil then fails = 0 end
if locked_at == nil then locked_at = 0 end

local now_ms = tonumber(ARGV[1])
local lock_ttl_ms = tonumber(ARGV[2])
local max_failures = tonumber(ARGV[3])

if locked_at > 0 and (now_ms - locked_at) < lock_ttl_ms then
  -- 仍锁中 → 不修改
  return {0, fails}
end

fails = fails + 1
local triggered = 0
if fails >= max_failures then
  locked_at = now_ms
  fails = 0
  triggered = 1
end

redis.call('HSET', KEYS[1], 'fails', fails, 'locked_at', locked_at)
redis.call('PEXPIRE', KEYS[1], lock_ttl_ms)
return {triggered, fails}
`)

// splitVal 拆分 "code|lastSentMs" 格式
func splitVal(s string) []string {
	for j := 0; j < len(s); j++ {
		if s[j] == '|' {
			return []string{s[:j], s[j+1:]}
		}
	}
	return []string{s}
}

// 编译期断言：RedisStore 实现 LoginLockStore 接口
var _ LoginLockStore = (*RedisStore)(nil)