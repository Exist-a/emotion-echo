// Package memory — summary_store.go
//
// CL-08a · A3：摘要的 Redis 写侧（best-effort）。
//
// 关键语义：**Redis 挂不得影响抽取落库** —— 摘要是增强不是依赖
// （与 CL-D-07 的写侧纪律一致）。因此这里只返回 error，由调用方记日志/指标。
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/emotion-echo/shared/pkg/memorykeys"
	"github.com/redis/go-redis/v9"
)

// SummaryStore 摘要写侧（便于单测注入假实现）。
type SummaryStore interface {
	Set(ctx context.Context, userID int64, summary string) error
}

// stringSetter 把 go-redis 收窄成两个方法，避免单测依赖真实连接。
type stringSetter interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

type redisStringSetter struct{ client *redis.Client }

func (s redisStringSetter) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s redisStringSetter) Del(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// RedisSummaryStore 生产实现：写 `memory:summary:user:<id>`，TTL 24h。
type RedisSummaryStore struct {
	setter stringSetter
	ttl    time.Duration
}

func NewRedisSummaryStore(client *redis.Client) *RedisSummaryStore {
	return &RedisSummaryStore{setter: redisStringSetter{client: client}, ttl: memorykeys.SummaryTTL}
}

// newSummaryStoreWithSetter 供单测注入假 setter。
func newSummaryStoreWithSetter(setter stringSetter, ttl time.Duration) *RedisSummaryStore {
	if ttl <= 0 {
		ttl = memorykeys.SummaryTTL
	}
	return &RedisSummaryStore{setter: setter, ttl: ttl}
}

func (s *RedisSummaryStore) Set(ctx context.Context, userID int64, summary string) error {
	if summary == "" {
		// 空摘要 = 没有可注入的记忆 ⇒ 删除键，避免读到旧值。
		return s.setter.Del(ctx, memorykeys.SummaryKey(userID))
	}
	return s.setter.Set(ctx, memorykeys.SummaryKey(userID), summary, s.ttl)
}

// InMemorySummaryStore 测试替身。
type InMemorySummaryStore struct {
	mu   sync.RWMutex
	data map[int64]string
	Err  error
}

func NewInMemorySummaryStore() *InMemorySummaryStore {
	return &InMemorySummaryStore{data: make(map[int64]string)}
}

func (s *InMemorySummaryStore) Set(ctx context.Context, userID int64, summary string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[userID] = summary
	return nil
}

func (s *InMemorySummaryStore) Get(userID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[userID]
	return v, ok
}
