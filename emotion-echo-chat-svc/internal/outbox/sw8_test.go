package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== CL-03 Phase 2：写入 outbox 行时把当前 ctx 的 sw8 存下来 =====
//
// 上半段（写侧）。下半段（relay 恢复父 ctx）见 relay_trace_test.go。

// sw8StubTracer 是 grpcinterceptor.Tracer 的最小替身，只关心 EncodeSW8。
type sw8StubTracer struct {
	sw8       string
	err       error
	opNameGot string
}

func (t *sw8StubTracer) StartEntry(ctx context.Context, _ string) (context.Context, grpcinterceptor.Span) {
	return ctx, &sw8StubSpan{}
}
func (t *sw8StubTracer) CreateLocalSpan(ctx context.Context, _ string) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &sw8StubSpan{}, nil
}
func (t *sw8StubTracer) CreateExitSpan(ctx context.Context, _, _ string, _ func(string, string) error) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &sw8StubSpan{}, nil
}
func (t *sw8StubTracer) CreateEntrySpan(ctx context.Context, _ string, _ func(string) (string, error)) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &sw8StubSpan{}, nil
}
func (t *sw8StubTracer) EncodeSW8(_ context.Context, opName string) (string, error) {
	t.opNameGot = opName
	return t.sw8, t.err
}

type sw8StubSpan struct{}

func (*sw8StubSpan) EndSpan(error)      {}
func (*sw8StubSpan) Tag(string, string) {}
func (*sw8StubSpan) SetSpanLayer(int32) {}
func (*sw8StubSpan) SetComponent(int32) {}

var _ grpcinterceptor.Tracer = (*sw8StubTracer)(nil)

// withGlobalTracer 临时替换全局 tracer 并在用例结束复原（EncodeTraceSW8 读的是全局，
// 与 CL-F-14「请求期现读全局」一致 ⇒ 后台补挂的 tracer 也能生效）。
func withGlobalTracer(t *testing.T, tr grpcinterceptor.Tracer) {
	t.Helper()
	prev := grpcinterceptor.GlobalTracer()
	grpcinterceptor.SetGlobalTracer(tr)
	t.Cleanup(func() { grpcinterceptor.SetGlobalTracer(prev) })
}

// TestEncodeTraceSW8_WithGlobalTracer_ReturnsEncodedValue 有 tracer ⇒ 返回编码值。
func TestEncodeTraceSW8_WithGlobalTracer_ReturnsEncodedValue(t *testing.T) {
	const want = "1-ZW52LTE-1-2-0-chat-svc-inst-POST /api/v1/messages-1.2.3.4:80"
	tr := &sw8StubTracer{sw8: want}
	withGlobalTracer(t, tr)

	got := EncodeTraceSW8(context.Background(), "outbox-enqueue")

	assert.Equal(t, want, got, "有 tracer 时必须原样返回编码出的 sw8")
	assert.Equal(t, "outbox-enqueue", tr.opNameGot, "operationName 应透传给 tracer（便于在 trace 里识别）")
}

// TestEncodeTraceSW8_NoTracer_ReturnsEmpty 无 tracer（SkyWalking 关闭）⇒ 空串、不报错。
func TestEncodeTraceSW8_NoTracer_ReturnsEmpty(t *testing.T) {
	withGlobalTracer(t, nil)
	assert.Empty(t, EncodeTraceSW8(context.Background(), "outbox-enqueue"),
		"无 tracer 必须返回空串 —— 调用方据此不写列，业务零影响")
}

// TestEncodeTraceSW8_TracerError_ReturnsEmpty 编码失败 ⇒ 降级空串（绝不让埋点失败影响落库）。
func TestEncodeTraceSW8_TracerError_ReturnsEmpty(t *testing.T) {
	withGlobalTracer(t, &sw8StubTracer{sw8: "partial", err: errors.New("encode boom")})
	assert.Empty(t, EncodeTraceSW8(context.Background(), "outbox-enqueue"),
		"编码失败必须降级为空串（trace 埋点不得影响业务写库）")
}

// TestEncodeTraceSW8_NilCtx_DoesNotPanic 防御：nil ctx 不得 panic。
func TestEncodeTraceSW8_NilCtx_DoesNotPanic(t *testing.T) {
	tr := &sw8StubTracer{sw8: "x"}
	withGlobalTracer(t, tr)
	require.NotPanics(t, func() { _ = EncodeTraceSW8(nil, "op") })
}
