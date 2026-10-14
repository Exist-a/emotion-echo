// Package trace — background_retry.go
//
// CL-F-14：**启动期 tracer 初始化失败后的后台补挂**。
//
// 问题（2026-10-14 起栈实测）：svc 与 SkyWalking OAP 常同时启动，svc 侧
// `BootstrapSkyWalkingTracerRetry`（5 次 × 2s ≈ 10s 窗口）在 OAP 的 :11800
// 尚未 listen 时会**整体失败**；既有行为是「打 warn 继续启动，进程余生不再重试」
// ⇒ 该服务在 SkyWalking 服务列表里**整段生命周期缺席**
// （实测 `getAllServices` 5 个，应有 7 个；`docker restart` 即愈）。
//
// 本文件把「一次性重试」升级为「后台持续重试直到成功」：
//   - 成功 → 调本包 SetTracer（它会一并注册 grpcinterceptor 全局槽，
//     入口路径因此能按请求现读，见 grpcinterceptor/global_tracer.go）
//   - ctx 取消 / 调 stop() → 立即退出（不泄漏 goroutine）
//   - 退避从 InitialBackoff 指数增长到 MaxBackoff 封顶（默认 5s → 60s）
//
// **不阻塞启动**：调用方拿到 stop 函数后立刻继续，服务照常对外提供能力，
// 只是这段时间内没有链路数据。
package trace

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/SkyAPM/go2sky"

	"github.com/emotion-echo/shared/pkg/bootstrap"
	"github.com/emotion-echo/shared/pkg/logging"
)

// errTracerNil 初始化返回 (nil, nil) 时的兜底错误（不该发生，但别当成功）。
var errTracerNil = errors.New("bootstrap returned nil tracer without error")

// TracerBootstrapFn 单次 tracer 初始化（可注入，便于单测不依赖真实 OAP）。
type TracerBootstrapFn func(ctx context.Context, serviceName, oapAddr string, dialTimeout time.Duration) (*go2sky.Tracer, error)

// tracerBootstrapFn 默认用 shared/bootstrap 的 TCP 探活 + reporter 实现。
// 声明为包级变量是为了让单测替换成确定性 stub（生产路径不变）。
var tracerBootstrapFn TracerBootstrapFn = bootstrap.BootstrapSkyWalkingTracer

// TracerBackgroundRetryOptions 后台补挂的节奏参数。零值即默认值。
type TracerBackgroundRetryOptions struct {
	// DialTimeout 单次尝试的 TCP 探活超时（默认 2s）。
	DialTimeout time.Duration
	// InitialBackoff 首次失败后的退避（默认 5s）。
	InitialBackoff time.Duration
	// MaxBackoff 退避上限（默认 60s）。
	MaxBackoff time.Duration
	// MaxAttempts 尝试次数上限；<= 0 表示不限次（直到成功或 ctx 取消）。
	MaxAttempts int
}

func (o TracerBackgroundRetryOptions) withDefaults() TracerBackgroundRetryOptions {
	if o.DialTimeout <= 0 {
		o.DialTimeout = 2 * time.Second
	}
	if o.InitialBackoff <= 0 {
		o.InitialBackoff = 5 * time.Second
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 60 * time.Second
	}
	if o.MaxBackoff < o.InitialBackoff {
		o.MaxBackoff = o.InitialBackoff
	}
	return o
}

// StartTracerBackgroundRetry 起一个后台 goroutine 反复尝试初始化 tracer，成功即 SetTracer。
//
// 返回的 stop 幂等，可安全多次调用；ctx 取消同样会让 goroutine 退出。
func StartTracerBackgroundRetry(ctx context.Context, serviceName, oapAddr string, opts TracerBackgroundRetryOptions) (stop func()) {
	o := opts.withDefaults()
	inner, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel() // 无论何种原因退出，都释放 inner（stop 只是触发 cancel）
		backoff := o.InitialBackoff
		for attempt := 1; o.MaxAttempts <= 0 || attempt <= o.MaxAttempts; attempt++ {
			// 首次立即尝试：启动期的失败已由调用方记过一次，这里不重复等待。
			t, err := tracerBootstrapFn(inner, serviceName, oapAddr, o.DialTimeout)
			if err == nil && t != nil {
				SetTracer(t)
				logging.PrintfContext(inner, "[skywalking] tracer 后台补挂成功（service=%s attempt=%d）—— 入口 span 自此刻起生效", serviceName, attempt)
				return
			}
			if err == nil {
				err = errTracerNil
			}
			logging.PrintfContext(inner, "[skywalking] tracer 后台补挂第 %d 次失败（service=%s backoff=%s）：%v", attempt, serviceName, backoff, err)

			select {
			case <-inner.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < o.MaxBackoff {
				backoff *= 2
				if backoff > o.MaxBackoff {
					backoff = o.MaxBackoff
				}
			}
		}
		logging.PrintfContext(inner, "[skywalking] tracer 后台补挂已达尝试上限 %d 次，放弃（service=%s）", o.MaxAttempts, serviceName)
	}()

	var once sync.Once
	return func() { once.Do(cancel) }
}
