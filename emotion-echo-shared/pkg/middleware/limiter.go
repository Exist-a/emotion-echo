// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件（Gin 版本）
//
// Stage 25-G: UserRateLimitMiddleware 提供 per-user 令牌桶限流
//
// 设计动机：
//   - 防止单个用户打爆 ai-svc 的 LLM 调用
//   - 不同用户之间隔离（恶意用户不影响正常用户）
//   - 内存级实现，无需 Redis（适合单机部署）
//
// 算法：token bucket
//   - 每用户独立桶（key=userID）
//   - 桶容量 = burst（瞬时允许的并发数）
//   - refill rate = rate tokens/秒
//   - 每次请求消耗 1 token，桶空则 429
//
// 注意：
//   - 本实现是 in-memory 限流，不适用于多实例部署
//   - 多实例应使用 Redis token bucket（如 redis-cell 模块）
package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// _ = net/http 用于 IPRateLimitMiddleware 编译期引用（防止改 import 时遗漏）
var _ = http.StatusTooManyRequests

// TokenBucket 内存级令牌桶（按 key 维度，如 userID）
type TokenBucket struct {
	rate    float64       // refill rate (tokens/sec)
	burst   float64       // 桶容量
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

// NewTokenBucket 创建令牌桶
//
// 参数：
//   - ratePerSec: 每秒补充的 token 数（可小数，如 0.5 = 每 2s 一个 token）
//   - burst: 桶容量（最大瞬时并发）
func NewTokenBucket(ratePerSec float64, burst int) *TokenBucket {
	tb := &TokenBucket{
		rate:    ratePerSec,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
	}
	// P1-18 (Round 1): buckets map 无清理 → 长跑 OOM。
	// 启动后台 goroutine 每 5 分钟扫一次，剔除 idle > 10 分钟的桶。
	// idle 阈值 = max(refill_time, 10min) —— refill_time 是补满桶需要的时间。
	refillMinutes := int(burst)/int(ratePerSec*60) + 1
	if refillMinutes < 10 {
		refillMinutes = 10
	}
	go tb.gcLoop(time.Duration(refillMinutes) * time.Minute)
	return tb
}

// gcLoop P1-18：周期清理 idle bucket
func (t *TokenBucket) gcLoop(idleThreshold time.Duration) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-idleThreshold)
		t.mu.Lock()
		for k, b := range t.buckets {
			if b.lastFill.Before(cutoff) {
				delete(t.buckets, k)
			}
		}
		t.mu.Unlock()
	}
}

// Allow 检查 key 是否能通过（消耗 1 个 token）
func (t *TokenBucket) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	b, ok := t.buckets[key]
	if !ok {
		// 新用户：满桶
		t.buckets[key] = &bucket{tokens: t.burst - 1, lastFill: now}
		return true
	}

	// refill: 距离上次 now 经过的时间 × rate
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * t.rate
	if b.tokens > t.burst {
		b.tokens = t.burst
	}
	b.lastFill = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RetryAfter 计算 key 需要等待多少秒才能再次通过
func (t *TokenBucket) RetryAfter(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	b, ok := t.buckets[key]
	if !ok {
		return 0
	}
	if b.tokens >= 1 {
		return 0
	}
	// 需要 (1 - tokens) / rate 秒才能补到 1
	need := 1 - b.tokens
	secs := need / t.rate
	if secs < 1 {
		secs = 1 // 最小返回 1s
	}
	return time.Duration(secs * float64(time.Second))
}

// LimiterBackend Round 4.3 PR-2：限流后端抽象接口。
//
// 当前实现：
//   - InMemoryLimiterBackend：单进程内存限流（默认，单副本部署）
//   - RedisLimiterBackend：TODO 多副本共享限流（触发条件=多副本部署）
//
// 设计目的：业务代码只依赖接口，部署形态决定 backend。
// 切换方式：LIMITER_BACKEND=inmemory|redis env（main.go 读取后 NewXxxBackend）。
type LimiterBackend interface {
	// Allow 检查 key 是否通过（消耗 1 个 token）
	Allow(key string) bool
	// RetryAfter 计算 key 需要等待多久才能再次通过
	RetryAfter(key string) time.Duration
}

// 编译期断言：TokenBucket 实现 LimiterBackend 接口（in-memory 默认 backend）
var _ LimiterBackend = (*TokenBucket)(nil)

// UserIDExtractor 从 gin.Context 提取 user_id 的回调
type UserIDExtractor func(*gin.Context) int64

// UserRateLimitMiddleware 返回限流中间件
//
// 参数：
//   - tb: 令牌桶
//   - getUserID: 从 ctx 取 user_id 的函数（通常用 testGetUserID 或自己的版本）
//
// 行为：
//   - user_id == 0（未鉴权）→ 跳过限流（让上游 auth 处理）
//   - 桶空 → 返回 429 + Retry-After 头
func UserRateLimitMiddleware(tb *TokenBucket, getUserID UserIDExtractor) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := getUserID(c)
		if uid == 0 {
			// 没 user_id（未鉴权），让上游处理
			c.Next()
			return
		}

		key := strconv.FormatInt(uid, 10)
		if tb.Allow(key) {
			c.Next()
			return
		}

		// 被限流
		retry := tb.RetryAfter(key)
		c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":       "rate limit exceeded",
			"retry_after": int(retry.Seconds()),
		})
	}
}

// IPRateLimitMiddleware Round 4.5 PR-3：per-IP 令牌桶限流。
//
// 用途：anonymous / 未鉴权请求也能被限流（防 DoS）。
// 与 UserRateLimitMiddleware 互补：UserRateLimit 处理已登录用户滥用，
// IPRateLimit 处理匿名 + 同 IP 多用户滥用。
//
// 实现：复用 TokenBucket，key=client IP（从 c.ClientIP() 拿，已被 Gin
// 信任代理链解析——若不可信可改 c.Request.RemoteAddr）。
func IPRateLimitMiddleware(tb *TokenBucket) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			c.Next() // 无 IP 时不阻断（极端兜底）
			return
		}
		if tb.Allow(ip) {
			c.Next()
			return
		}
		retry := tb.RetryAfter(ip)
		c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":       "ip rate limit exceeded",
			"retry_after": int(retry.Seconds()),
		})
	}
}