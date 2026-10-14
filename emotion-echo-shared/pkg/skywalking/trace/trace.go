// Package trace 提供**无重依赖**（仅 go2sky）的全局 tracer 注册与 span 助手。
//
// 拆分缘由（CL-01 G3 / D-04）：
//   - 原 `pkg/skywalking` 包同时含 GORM / Redis 埋点（import gorm.io/gorm、
//     redis/go-redis）—— 任何 import 都会把这些依赖拖进调用方的模块图。
//   - web-bff（API 网关）只想用 tracer + HTTP 出站埋点，**不该因此依赖 ORM**
//     （实测：BFF go.sum 里 gorm 条目数 = 0，import skywalking 会新拉入 gorm）。
//   - 故把「全局注册表 + exit-span 助手 + HTTP RoundTripper」拆到本包；
//     `pkg/skywalking` 保留同名 forwarder（SetTracer/Tracer），**既有 5 个 svc 零改动**。
package trace

import (
	"context"
	"sync"
	"time"

	"github.com/SkyAPM/go2sky"
)

var (
	mu     sync.RWMutex
	tracer *go2sky.Tracer
)

// SetTracer 注册（或清空）全局 tracer；传 nil 清空。
//
// 线程安全：启动期写、请求期读，RWMutex 兜底 -race 门禁。
func SetTracer(t *go2sky.Tracer) {
	mu.Lock()
	tracer = t
	mu.Unlock()
}

// Tracer 返回全局 tracer，未注册时返回 nil。
func Tracer() *go2sky.Tracer {
	mu.RLock()
	defer mu.RUnlock()
	return tracer
}

// ContextOrBg 把 nil ctx 换成 context.Background()
// （go2sky 与 ctx.Value 都不接受 nil）。
func ContextOrBg(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// CreateExitSpan 包装一次外部组件调用（GORM / Redis / 外部 HTTP）的 exit span。
//
// 入参:
//   - ctx: 上下文（应已带 trace 信息；GORM 走 db.Statement.Context，Redis hook 走 ctx）
//   - tgr: tracer（来自 Tracer()）
//   - name: 操作名（如 "gorm.query" / "redis.GET"）
//   - peer: 对端标识（"postgres:5432" / "redis:6379"）
//
// 返回 end 闭包；调用方在合适时机调用。无 tracer / 创建失败时返回 noop
// （不会破坏调用方逻辑）。
//
// 边界（CL-01 约束③ / T-08）：**仅当 ctx 已带父 span 时才建 span** —— go2sky
// 默认采样率=1.0（NewDynamicSampler(1)），ctx 无父 span 时会新建"根 trace"
//（硬造根 span），为无 ctx 的调用（后台任务等）刷出大量孤立 trace，故直接 noop。
func CreateExitSpan(ctx context.Context, tgr *go2sky.Tracer, name, peer string) func(...EndOption) {
	if tgr == nil || name == "" || peer == "" {
		return func(...EndOption) {}
	}
	ctx = ContextOrBg(ctx)
	if go2sky.ActiveSpan(ctx) == nil {
		return func(...EndOption) {}
	}

	span, _, err := tgr.CreateExitSpanWithContext(ctx, name, peer, func(_, _ string) error {
		return nil // no-op injector；需要 propagation 时由调用方（如 HTTP RoundTripper）自行注入
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

// EndOption 是 CreateExitSpan 返回 end 函数的参数
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
