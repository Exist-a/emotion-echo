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

	// 不能只依赖 ctx 传播中止读：黑洞场景（连接通、永不回包）实测 ctx deadline
	// 拦不住 go-redis 在**池化连接初始化**上的阻塞，会拖到默认 ReadTimeout 3s
	//（CL-08c Phase 3 实测抓到；300ms 契约见上方常量注释）。故用 select 兜底：
	// 到点必返；慢查询留给后台 goroutine 自行超时回收（有界、仅病态路径发生）。
	type result struct {
		val string
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := s.client.Get(readCtx, sharedmemorykeys.SummaryKey(userID)).Result()
		done <- result{val: v, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			if errors.Is(r.err, redis.Nil) {
				// 没有记忆是正常状态，不是错误。
				return "", nil
			}
			return "", r.err
		}
		return r.val, nil
	case <-readCtx.Done():
		return "", readCtx.Err()
	}
}
