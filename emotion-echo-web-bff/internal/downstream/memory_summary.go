// Package downstream — memory_summary.go
//
// CL-08a · A4：记忆摘要的读取侧（BFF）。
//
// 契约：只读 Redis 的 `memory:summary:user:<id>`（ai-svc 写入）。
// 取不到 / 报错 / 为空 ⇒ 调用方传空串，对话照常（记忆是增强不是依赖）。
package downstream

import (
	"context"
	"errors"

	sharedmemorykeys "github.com/emotion-echo/shared/pkg/memorykeys"
	"github.com/redis/go-redis/v9"
)

// MemorySummarySource 用户级记忆摘要来源。
type MemorySummarySource interface {
	LatestMemorySummary(ctx context.Context, userID int64) (string, error)
}

type redisMemorySummary struct{ client *redis.Client }

// NewRedisMemorySummary client 为 nil 时返回 nil（= 未装配，注入侧自然降级）。
func NewRedisMemorySummary(client *redis.Client) MemorySummarySource {
	if client == nil {
		return nil
	}
	return &redisMemorySummary{client: client}
}

func (s *redisMemorySummary) LatestMemorySummary(ctx context.Context, userID int64) (string, error) {
	v, err := s.client.Get(ctx, sharedmemorykeys.SummaryKey(userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 没有记忆是正常状态，不是错误。
			return "", nil
		}
		return "", err
	}
	return v, nil
}
