package skywalking

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

// InstrumentRedis 给 *redis.Client 装上 Tracing Hook，让每条 Redis 命令产生一个 ExitSpan。
//
// 用法（仅需一次）：
//
//	rdb := database.NewRedis(...)
//	skywalking.InstrumentRedis(rdb)
func InstrumentRedis(rdb *redis.Client) {
	// 防御：nil client 不挂 hook（以前会 panic；Stage 26-A 暴露 bug #2）
	if rdb == nil {
		return
	}
	rdb.AddHook(&redisTracingHook{addr: rdb.Options().Addr})
}

// redisTracingHook 实现 go-redis Hook 接口
type redisTracingHook struct {
	addr string
}

// DialHook 连接阶段：通常无 trace 必要，直接放行
func (h *redisTracingHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

// ProcessHook 单条命令的 hook（GET/SET/DEL 等）
func (h *redisTracingHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		tgr := Tracer()
		if tgr == nil {
			return next(ctx, cmd)
		}
		ctx2 := contextOrBg(ctx)
		end := createExitSpan(ctx2, tgr, "redis."+cmd.Name(), "redis@"+h.addr)

		err := next(ctx2, cmd)
		if err != nil && !errors.Is(err, redis.Nil) {
			end(WithError(err))
		} else {
			end()
		}
		return err
	}
}

// ProcessPipelineHook pipeline 命令（一条 TCP 链多个命令）
func (h *redisTracingHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		tgr := Tracer()
		if tgr == nil {
			return next(ctx, cmds)
		}
		ctx2 := contextOrBg(ctx)
		end := createExitSpan(ctx2, tgr, "redis.pipeline", "redis@"+h.addr)

		names := make([]string, 0, len(cmds))
		for _, c := range cmds {
			names = append(names, c.Name())
		}
		// 把命令列表作为 tag 写到 span
		err := next(ctx2, cmds)
		if err != nil {
			end(WithError(err), WithTag("redis.cmds", joinCmds(names)))
		} else {
			end(WithTag("redis.cmds", joinCmds(names)))
		}
		return err
	}
}

func joinCmds(names []string) string {
	if len(names) == 0 {
		return ""
	}
	out := names[0]
	for i := 1; i < len(names); i++ {
		out += "," + names[i]
	}
	return out
}

// 确保 go-redis Hook 接口完整实现
var _ redis.Hook = (*redisTracingHook)(nil)
