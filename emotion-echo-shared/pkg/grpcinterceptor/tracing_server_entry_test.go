// E2E-26 测试点 #3：gRPC server 端必须用 CreateEntrySpan 从 incoming metadata 提取 sw8，
// 并把承载 span 的 ctx 交给 handler —— 否则跨进程 trace 每跳各起新 trace。
//
// 根因（2026-10-02 E2E-26 执行期实测）：NewServerTracingInterceptor 调的是
// StartEntry（go2sky 侧实现 = CreateExitSpanWithContext 空注入 + 返回**原 ctx**），
// 既不提取上游 sw8 也不把 span 挂进 handler ctx ⇒ OAP 实测 user-svc 只有
// /health/ready span、RPC 全部缺席、trace 树无法跨进程拼接。
package grpcinterceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type serverEntryCtxKey struct{}

// TestServerTracing_UsesCreateEntrySpan_ExtractsSw8FromIncomingMetadata
// server 拦截器必须走 CreateEntrySpan：extractor 读 incoming metadata 的 sw8，
// handler 拿到 CreateEntrySpan 返回的承载 ctx。
func TestServerTracing_UsesCreateEntrySpan_ExtractsSw8FromIncomingMetadata(t *testing.T) {
	marked := context.WithValue(context.Background(), serverEntryCtxKey{}, "entry-ctx")
	tr := &mockTracer{entryCtx: marked, entrySpan: &mockSpan{}}
	interceptor := NewServerTracingInterceptor(tr)

	const sw8Val = "1-abc123-0-segfro-7-svc-inst-end-1"
	md := metadata.Pairs("sw8", sw8Val)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/emotion_test.v1.Svc/Do"}
	var gotCtx context.Context
	handler := func(c context.Context, req interface{}) (interface{}, error) {
		gotCtx = c
		return "ok", nil
	}

	resp, err := interceptor(ctx, "req-payload", info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	assert.Equal(t, []string{"/emotion_test.v1.Svc/Do"}, tr.entryOpCalls,
		"server 端必须用 CreateEntrySpan（带 sw8 extractor），而不是 StartEntry——"+
			"StartEntry 不提取 sw8 ⇒ 跨进程 trace 断链（E2E-26 #3）")
	assert.Equal(t, sw8Val, tr.entrySw8Seen,
		"extractor 必须能从 incoming metadata 读到 sw8 值")
	assert.True(t, gotCtx == marked,
		"handler 必须拿到 CreateEntrySpan 返回的承载 ctx——否则下游 exit span 续不上同一 trace")
}

// TestServerTracing_NoSw8Metadata_HandlerStillReceivesEntryCtx 无 sw8（根请求）时
// 仍走 CreateEntrySpan 且不伪造父引用（extractor 返回空 → go2sky 视为 root entry）。
func TestServerTracing_NoSw8Metadata_HandlerStillReceivesEntryCtx(t *testing.T) {
	marked := context.WithValue(context.Background(), serverEntryCtxKey{}, "root-entry")
	tr := &mockTracer{entryCtx: marked, entrySpan: &mockSpan{}}
	interceptor := NewServerTracingInterceptor(tr)

	info := &grpc.UnaryServerInfo{FullMethod: "/no.sw8/Bare"}
	var gotCtx context.Context
	handler := func(c context.Context, req interface{}) (interface{}, error) {
		gotCtx = c
		return nil, nil
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)

	require.Equal(t, []string{"/no.sw8/Bare"}, tr.entryOpCalls, "无 sw8 也必须建 entry span")
	assert.Equal(t, "", tr.entrySw8Seen, "无 metadata 时 extractor 应读到空（不伪造）")
	assert.True(t, gotCtx == marked, "handler 必须拿到承载 ctx")
}
