// E2E-26 #3 诊断+契约：ClientTracingInterceptor 必须把 go2sky 编码的 sw8
// 写进 outgoing metadata（真实 go2sky tracer，非 mock —— mock 只能证明
// 自己被调用，证明不了 go2sky Encode 管线真的产出 sw8）。
//
// 背景（2026-10-03 运行时实测）：服务端探针（伪造 sw8 直打 user-svc）实证
// 服务端解码正常（CROSS_PROCESS ref 挂上），但 BFF 发起的 GetMe 在 user-svc
// 侧是 root trace ⇒ sw8 没到线上。本测试分离责任：若本测试红 ⇒ 客户端
// 注入管线（interceptor × go2sky）有 bug；若绿 ⇒ 差异在 BFF 装配/传输层。
package grpcinterceptor

import (
	"context"
	"strings"
	"testing"

	"github.com/SkyAPM/go2sky"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeReporter：go2sky 无 reporter 时 initFlag 恒 0 ⇒ 全 NoopSpan（连 sw8 都
// 不注入，trace.go:188 "Ignored, there is no need to inject"）——测试必须带
// reporter 才反映生产形态（Bootstrap 走 NewGRPCReporter ⇒ initFlag=1）。
type fakeReporter struct{}

func (fakeReporter) Boot(string, string, []go2sky.AgentConfigChangeWatcher) {}
func (fakeReporter) Send([]go2sky.ReportedSpan)                             {}
func (fakeReporter) Close()                                                 {}

func TestClientTracingInterceptor_InjectsSw8IntoOutgoingMetadata(t *testing.T) {
	realTr, err := go2sky.NewTracer("client-md-test", go2sky.WithReporter(fakeReporter{}))
	if err != nil {
		t.Skipf("go2sky.NewTracer unsupported: %v", err)
	}
	ic := NewClientTracingInterceptor(NewGo2SkyTracer(realTr))

	var gotMD metadata.MD
	var gotMethod string
	invoker := func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		gotMethod = method
		gotMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	// 模拟 gin 入口 span 承载的 ctx（E2E-26 修复后的生产形态）
	pctx, _ := NewGo2SkyTracer(realTr).StartEntry(context.Background(), "GET /api/test")
	require.NotNil(t, pctx)

	err = ic(pctx, "/emotion_user.v1.UserService/GetMe", nil, nil, nil, invoker)
	require.NoError(t, err)
	require.Equal(t, "/emotion_user.v1.UserService/GetMe", gotMethod)

	require.NotNil(t, gotMD, "invoker 必须收到带 outgoing metadata 的 ctx")
	sw8Vals := gotMD.Get("sw8")
	require.NotEmpty(t, sw8Vals, "outgoing metadata 缺 sw8 —— go2sky Encode 未注入或被后续拦截器丢弃")
	sw8 := sw8Vals[0]
	parts := strings.Split(sw8, "-")
	assert.GreaterOrEqual(t, len(parts), 8, "sw8 应为 8 段格式，got: %s", sw8)
	assert.Equal(t, "1", parts[0], "sw8 sampling 段")
}
