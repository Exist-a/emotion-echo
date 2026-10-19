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
	"time"

	sharedmemorykeys "github.com/emotion-echo/shared/pkg/memorykeys"
	"github.com/redis/go-redis/v9"
)

// memorySummaryReadTimeout 摘要读取的硬超时。
//
// 记忆是**增强不是依赖**：Redis 不可达时不能把首字延迟拖到 go-redis 的默认
// DialTimeout（5s）—— 那会让每次对话白等 5 秒。300ms 内取不到就当没有记忆。
const memorySummaryReadTimeout = 300 * time.Millisecond

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
	readCtx, cancel := context.WithTimeout(ctx, memorySummaryReadTimeout)
	defer cancel()
	v, err := s.client.Get(readCtx, sharedmemorykeys.SummaryKey(userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 没有记忆是正常状态，不是错误。
			return "", nil
		}
		return "", err
	}
	return v, nil
}
