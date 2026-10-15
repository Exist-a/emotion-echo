package grpcinterceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// CL-F-14 回归钉：全局 tracer 槽必须支持「先构造、后注册」。
//
// 复现的真实时序（2026-10-14 起栈实测）：svc 先于 OAP 启动 ⇒ 构造 gin 中间件 /
// gRPC server 拦截器时 tracer 还是 nil；OAP 起来后由后台补挂 SetTracer。
// 若入口路径把 nil 快照进闭包，补挂永远进不来 ⇒ 该服务整段生命周期零 span。
func TestGlobalTracer_SetAndGet(t *testing.T) {
	SetGlobalTracer(nil)
	t.Cleanup(func() { SetGlobalTracer(nil) })

	assert.Nil(t, GlobalTracer(), "未注册时应为 nil")

	tr := &mockTracer{}
	SetGlobalTracer(tr)
	assert.Same(t, tr, GlobalTracer())

	SetGlobalTracer(nil)
	assert.Nil(t, GlobalTracer(), "清空后应回到 nil")
}

// TestServerTracingInterceptor_ReadsGlobalTracer_WhenConstructedWithNil
// 构造期 tracer=nil，之后注册全局 ⇒ 仍必须建入口 span。
func TestServerTracingInterceptor_ReadsGlobalTracer_WhenConstructedWithNil(t *testing.T) {
	SetGlobalTracer(nil)
	t.Cleanup(func() { SetGlobalTracer(nil) })

	// 构造期（模拟 svc 先于 OAP 启动）
	interceptor := NewServerTracingInterceptor(nil)

	// 之后后台补挂
	tr := &mockTracer{entrySpan: &mockSpan{}, entryCtx: context.Background()}
	SetGlobalTracer(tr)

	called := false
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	})
	require.NoError(t, err)
	assert.True(t, called, "handler 必须被调用")
	assert.Len(t, tr.entryOpCalls, 1,
		"补挂的全局 tracer 必须被入口拦截器读到（旧实现把 nil 快照进闭包 ⇒ 此处为 0）")
}

// TestServerTracingInterceptor_StillNoopWhenGlobalAlsoNil
// 负向对照：构造期 nil + 全局也 nil ⇒ 照旧 noop，不 panic、不建 span。
func TestServerTracingInterceptor_StillNoopWhenGlobalAlsoNil(t *testing.T) {
	SetGlobalTracer(nil)
	t.Cleanup(func() { SetGlobalTracer(nil) })

	interceptor := NewServerTracingInterceptor(nil)
	called := false
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}
