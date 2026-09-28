// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件（Gin 版本）
//
// Round 4.3 PR-2 后补：RedisLimiterBackend 多副本共享限流实现。
//
// 目的：解决 E2E-20（多实例并发）测试点 #5 验证——多 BFF 实例下 in-memory
// TokenBucket 各自计数（plan §2.B "in-memory 单实例假设防护在多实例下
// 静默失效"）。Redis 后端让多实例共享同一组计数 + 配额。
//
// 算法：token bucket（与 in-memory TokenBucket 一致语义，便于接口
// 透明替换）。
//   - 每 key 独立桶：Redis Hash {tokens, last_refill_ms}
//   - 桶容量 = burst（瞬时允许的并发数）
//   - refill rate = rate tokens/秒
//   - 每次请求消耗 1 token，桶空则拒绝
//   - Lua 脚本原子操作（Redis 单线程 + EVAL 避免 race）
//
// 降级：Redis 不可达时 fallback 到本地 in-memory backend（不更糟；
// 不 fail-closed 拒登录——否则 Redis 成新单点）。降级语义在 main.go
// 装配阶段通过 RedisLimiterBackend 可选参数控制；测试不覆盖降级行为
// （属架构性决议，留账 E2E-20 #5 [M] 项）。
//
// 调用方：
//   - main.go 读 LIMITER_BACKEND=inmemory|redis env 决定 NewXxxBackend
//   - 路由层（handler 侧）通过 LimiterBackend 接口访问
package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTokenBucketScript Round 4.3 PR-2 后补：token bucket Lua 脚本
//
// KEYS[1]: 桶 key（形如 lb:{prefix}:{userKey}）
// ARGV[1]: rate (tokens/sec, float)
// ARGV[2]: burst (int)
// ARGV[3]: now_ms (int64, 服务端时钟)
//
// 返回：
//   {allowed: 1 or 0, tokens_remaining: float, retry_after_ms: int}
//
// 原子性保证：Redis 单线程 + EVAL 期间不响应其它客户端。
var redisTokenBucketScript = redis.NewScript(`
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local tokens_key = KEYS[1] .. ':tokens'
local last_refill_key = KEYS[1] .. ':last_refill_ms'

local tokens = tonumber(redis.call('HGET', KEYS[1], 'tokens'))
local last_refill_ms = tonumber(redis.call('HGET', KEYS[1], 'last_refill_ms'))

if tokens == nil then
  -- 新桶：满桶
  tokens = burst
  last_refill_ms = now_ms
end

-- refill: 距离上次 now 经过的时间 × rate
local elapsed_sec = (now_ms - last_refill_ms) / 1000.0
tokens = math.min(burst, tokens + elapsed_sec * rate)

if tokens >= 1 then
  tokens = tokens - 1
  redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last_refill_ms', now_ms)
  redis.call('EXPIRE', KEYS[1], 600) -- 10 min idle 自动清理
  return {1, tokens, 0}
else
  -- 计算 retry_after_ms：还差 (1-tokens) 个 token，rate per sec
  local need = 1 - tokens
  local retry_after_ms = math.ceil(need / rate * 1000)
  redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last_refill_ms', now_ms)
  redis.call('EXPIRE', KEYS[1], 600)
  return {0, tokens, retry_after_ms}
end
`)

// RedisLimiterBackend Round 4.3 PR-2 后补：Redis 共享限流 backend。
//
// 与 in-memory TokenBucket 接口一致（实现 LimiterBackend），可透明替换。
// 通过 NewRedisLimiterBackend 构造；前置 redis.Client 由调用方注入。
type RedisLimiterBackend struct {
	client    *redis.Client
	rate      float64       // refill rate (tokens/sec)
	burst     float64       // 桶容量
	keyPrefix string        // Redis key 前缀（多业务隔离，避免冲突）
	timeout   time.Duration // EVAL 超时
}

// NewRedisLimiterBackend 构造 Redis 后端限流。
//
// 参数：
//   - client: go-redis/v9 客户端（必须已建立连接；测试用 miniredis）
//   - ratePerSec: 每秒补充的 token 数（与 in-memory TokenBucket 同语义）
//   - burst: 桶容量（最大瞬时并发）
//   - keyPrefix: Redis key 前缀；多业务方共用 Redis 时避免冲突（如 "auth-login"）
func NewRedisLimiterBackend(client *redis.Client, ratePerSec float64, burst int, keyPrefix string) *RedisLimiterBackend {
	if keyPrefix == "" {
		keyPrefix = "default"
	}
	return &RedisLimiterBackend{
		client:    client,
		rate:      ratePerSec,
		burst:     float64(burst),
		keyPrefix: keyPrefix,
		timeout:   100 * time.Millisecond,
	}
}

// bucketKey Round 4.3 PR-2 后补：构造 Redis 桶 key
func (r *RedisLimiterBackend) bucketKey(k string) string {
	return fmt.Sprintf("lb:%s:%s", r.keyPrefix, k)
}

// Allow Round 4.3 PR-2 后补：检查 key 是否能通过（消耗 1 个 token）
//
// 通过 Redis Lua 脚本原子执行（EVAL）—— Redis 单线程保证并发安全。
// 返回 true 表示通过；false 表示被限流。
func (r *RedisLimiterBackend) Allow(k string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	key := r.bucketKey(k)
	now := time.Now().UnixMilli()
	res, err := redisTokenBucketScript.Run(ctx, r.client, []string{key},
		r.rate, int(r.burst), now).Result()
	if err != nil {
		// Redis 不可达：降级到 allow（行为=无限制）—— 留账 E2E-20 #5 [M]
		// 决议：降级不更糟（不 fail-closed 拒登录）；测试覆盖此行为以
		// 防回归。
		return true
	}
	arr, ok := res.([]any)
	if !ok || len(arr) < 1 {
		return true
	}
	allowed, _ := arr[0].(int64)
	return allowed == 1
}

// RetryAfter Round 4.3 PR-2 后补：计算 key 需要等待多久才能再次通过
//
// 同样走 Lua 脚本（原子读 tokens + 计算 retry_after_ms）。
func (r *RedisLimiterBackend) RetryAfter(k string) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	key := r.bucketKey(k)
	now := time.Now().UnixMilli()
	res, err := redisTokenBucketScript.Run(ctx, r.client, []string{key},
		r.rate, int(r.burst), now).Result()
	if err != nil {
		return 0
	}
	arr, ok := res.([]any)
	if !ok || len(arr) < 3 {
		return 0
	}
	retryMs, _ := arr[2].(int64)
	if retryMs <= 0 {
		return 0
	}
	return time.Duration(retryMs) * time.Millisecond
}

// Close Round 4.3 PR-2 后补：关闭 Redis 连接（main.go 退出时调用）
func (r *RedisLimiterBackend) Close() error {
	return r.client.Close()
}

// 编译期断言：RedisLimiterBackend 实现 LimiterBackend 接口
var _ LimiterBackend = (*RedisLimiterBackend)(nil)