package trace

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SkyAPM/go2sky"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

// CL-F-14 回归钉：启动期失败后，后台补挂必须最终把 tracer 注册进**两个**全局槽。
//
// 复现的真实时序（2026-10-14 起栈实测）：svc 与 OAP 同起 ⇒ 启动期
// BootstrapSkyWalkingTracerRetry 的 ~10s 窗口内 OAP :11800 未 listen ⇒ 整体失败；
// 旧行为「打 warn 继续启动、余生不再重试」⇒ 该服务在 SkyWalking 服务列表缺席。
func TestStartTracerBackgroundRetry_SucceedsAfterTransientFailures(t *testing.T) {
	orig := tracerBootstrapFn
	t.Cleanup(func() { tracerBootstrapFn = orig; SetTracer(nil) })

	var attempts int32
	tr, err := go2sky.NewTracer("bg-retry-test", go2sky.WithReporter(&noopReporter{}))
	require.NoError(t, err)

	// 前 2 次失败（模拟 OAP 未就绪），第 3 次成功。
	tracerBootstrapFn = func(_ context.Context, _, _ string, _ time.Duration) (*go2sky.Tracer, error) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			return nil, errors.New("dial emotion-echo-sw-oap:11800: connect: connection refused")
		}
		return tr, nil
	}

	SetTracer(nil)
	stop := StartTracerBackgroundRetry(context.Background(), "bg-retry-test", "oap:11800", TracerBackgroundRetryOptions{
		DialTimeout:    time.Millisecond,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     2 * time.Millisecond,
	})
	defer stop()

	// 等补挂完成（最多 3s；退避是毫秒级）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if Tracer() != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	assert.GreaterOrEqual(t, atomic.LoadInt32(&attempts), int32(3), "应至少尝试 3 次")
	assert.NotNil(t, Tracer(), "补挂成功后 trace 全局槽必须有 tracer")
	assert.NotNil(t, grpcinterceptor.GlobalTracer(), "补挂成功后 grpcinterceptor 全局槽也必须有 tracer（入口路径靠它）")
}

// ctx 取消必须让后台 goroutine 退出（不泄漏），且不注册 tracer。
func TestStartTracerBackgroundRetry_StopsOnContextCancel(t *testing.T) {
	orig := tracerBootstrapFn
	t.Cleanup(func() { tracerBootstrapFn = orig; SetTracer(nil) })

	var attempts int32
	tracerBootstrapFn = func(_ context.Context, _, _ string, _ time.Duration) (*go2sky.Tracer, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("still down")
	}

	SetTracer(nil)
	ctx, cancel := context.WithCancel(context.Background())
	stop := StartTracerBackgroundRetry(ctx, "bg-retry-cancel", "oap:11800", TracerBackgroundRetryOptions{
		DialTimeout:    time.Millisecond,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
	})

	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(40 * time.Millisecond)
	after := atomic.LoadInt32(&attempts)
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, after, atomic.LoadInt32(&attempts), "cancel 后不应再有新的尝试")
	assert.Nil(t, Tracer(), "从未成功 ⇒ 不应注册 tracer")
	stop() // 幂等：cancel 已触发，重复调用不得 panic
	stop()
}

// MaxAttempts 封顶：失败到底后必须停止（不能无限刷日志）。
func TestStartTracerBackgroundRetry_HonoursMaxAttempts(t *testing.T) {
	orig := tracerBootstrapFn
	t.Cleanup(func() { tracerBootstrapFn = orig; SetTracer(nil) })

	var attempts int32
	tracerBootstrapFn = func(_ context.Context, _, _ string, _ time.Duration) (*go2sky.Tracer, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("nope")
	}

	SetTracer(nil)
	stop := StartTracerBackgroundRetry(context.Background(), "bg-retry-cap", "oap:11800", TracerBackgroundRetryOptions{
		DialTimeout:    time.Millisecond,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
		MaxAttempts:    3,
	})
	defer stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&attempts) >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts), "到达上限后必须停止尝试")
}

// SetTracer(nil) 必须把两个全局槽一起清空（否则优雅退出后会留悬挂引用）。
func TestSetTracer_NilClearsBothGlobalSlots(t *testing.T) {
	tr, err := go2sky.NewTracer("clear-test", go2sky.WithReporter(&noopReporter{}))
	require.NoError(t, err)
	SetTracer(tr)
	require.NotNil(t, Tracer())
	require.NotNil(t, grpcinterceptor.GlobalTracer())

	SetTracer(nil)
	assert.Nil(t, Tracer())
	assert.Nil(t, grpcinterceptor.GlobalTracer(), "grpcinterceptor 全局槽也必须清空")
}

// noopReporter 满足 go2sky.Reporter（不上报，避免单测连真实 OAP）。
type noopReporter struct{}

func (r *noopReporter) Boot(string, string, []go2sky.AgentConfigChangeWatcher) {}
func (r *noopReporter) Send([]go2sky.ReportedSpan)                               {}
func (r *noopReporter) Close()                                                   {}
