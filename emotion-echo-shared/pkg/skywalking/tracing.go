package skywalking

import (
	"context"
	"time"

	"github.com/SkyAPM/go2sky"
)

// createExitSpan 包装一次外部组件调用（GORM / Redis / 外部 HTTP）的 span。
//
// 入参:
//   - ctx: 上下文（应已带 trace 信息；GORM 走 db.Statement.Context，Redis hook 走 ctx）
//   - tgr: tracer（来自 Tracer()）
//   - name: 操作名（如 "gorm.query" / "redis.GET"）
//   - peer: 对端标识（"postgres:5432" / "redis:6379"）
//
// 返回:
//   - end 闭包；调用方在合适时机调用。
//     无 tracer 或创建失败时返回 noop（不会破坏调用方逻辑）
//
// 调用方必须在合适时机调用 end()；失败时可在 end 之前调 WithError / WithTag
func createExitSpan(ctx context.Context, tgr *go2sky.Tracer, name, peer string) func(...EndOption) {
	if tgr == nil {
		return func(...EndOption) {}
	}

	// Use CreateExitSpanWithContext 以拿到带 span 的 ctx
	span, _, err := tgr.CreateExitSpanWithContext(ctx, name, peer, func(_, _ string) error {
		return nil // no-op injector；如需 propagation 调 HTTP client 客户端时再注入
	})
	if err != nil || span == nil {
		return func(...EndOption) {}
	}
	return func(opts ...EndOption) {
		for _, opt := range opts {
			opt(span)
		}
		span.End()
	}
}

// EndOption 是 createExitSpan 返回 end 函数的参数
type EndOption func(s go2sky.Span)

// WithError 把 span 标记为 error
func WithError(err error) EndOption {
	return func(s go2sky.Span) {
		if err != nil {
			s.Error(time.Now(), err.Error())
		}
	}
}

// WithTag 在结束前补打 tag
func WithTag(key, value string) EndOption {
	return func(s go2sky.Span) {
		s.Tag(go2sky.Tag(key), value)
	}
}
