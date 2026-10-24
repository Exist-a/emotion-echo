// Package downstream — memory_summary_test.go
//
// CL-08c · Phase 3：真实 redisMemorySummary 的降级契约（收口 CL-F-70 ①）。
//
// CL-08a 只在 handler 层用 fake source 验过降级；本文件对**真实实现**钉死三个分支：
//   - 命中 ⇒ 原文返回（且用的 key 与 ai-svc 写侧同源 —— 跨服务契约）；
//   - 未命中（redis.Nil）⇒ ("", nil)：没有记忆是正常状态，不是错误；
//   - Redis 黑洞（连接通但永不回包）⇒ 300ms 硬超时内返回错误，
//     不得拖到 go-redis 默认 DialTimeout（5s）把首字延迟拖死。
package downstream

import (
	"context"
	"net"
	"testing"
	"time"

	sharedmemorykeys "github.com/emotion-echo/shared/pkg/memorykeys"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisMemorySummary_HitReturnsValue_SharedKeyContract(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	// 用 **shared 的 key 构造器**写入（与 ai-svc 写侧同源；抓"两侧 key 格式漂移"）
	want := "- 2026-10 用户是后端工程师\n- 2026-10 用户养了一只猫"
	require.NoError(t, client.Set(context.Background(),
		sharedmemorykeys.SummaryKey(7), want, 0).Err())

	src := NewRedisMemorySummary(client)
	got, err := src.LatestMemorySummary(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRedisMemorySummary_MissIsNotError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	src := NewRedisMemorySummary(client)
	got, err := src.LatestMemorySummary(context.Background(), 42)
	require.NoError(t, err, "redis.Nil 必须折叠为 (\"\", nil)——没有记忆是正常状态")
	assert.Equal(t, "", got)
}

// 黑洞端口：接受连接但永不回包 ⇒ Get 挂住 ⇒ 只能靠 LatestMemorySummary 自己的
// 300ms 超时脱身。若该超时被删/改大，本测试会在 2s 处拦下（远小于 go-redis
// 默认 5s DialTimeout——那会让每次无 Redis 的对话白等 5 秒）。
func TestRedisMemorySummary_BlackholeTimesOutFast(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c // 握手后挂住：不读不写不关
		}
	}()

	client := redis.NewClient(&redis.Options{Addr: ln.Addr().String()})
	defer func() { _ = client.Close() }()

	src := NewRedisMemorySummary(client)
	started := time.Now()
	got, err := src.LatestMemorySummary(context.Background(), 1)
	elapsed := time.Since(started)

	require.Error(t, err, "黑洞连接必须报错（由调用方降级为空）")
	assert.Equal(t, "", got)
	assert.Less(t, elapsed, 2*time.Second,
		"必须在 300ms 硬超时内返回（实测 %v）——记忆是增强不是依赖", elapsed)
}

// nil client ⇒ 返回 nil source（未装配 ⇒ 注入侧自然降级为空段）。
func TestNewRedisMemorySummary_NilClientReturnsNil(t *testing.T) {
	assert.Nil(t, NewRedisMemorySummary(nil))
}
