// tracing_sw8_encode_test.go
//
// CL-03 Phase 2/3 的**核心契约**：把"当前 ctx 所在 trace 的 sw8"取出来存进异步载体
// （outbox 行），之后用存下的 sw8 **恢复父 ctx**，两段必须在**同一条 trace** 上。
//
// 为什么需要新原语：relay 跑在后台根 ctx（chat-svc/main.go 的 relayCtx =
// context.WithCancel(context.Background())）⇒ kafka_publisher 的 CreateExitSpan
// 建的是「根」span，一条消息的 trace 断在 outbox→relay 处。
//
// 为什么只加**一个**方法（用户裁定 CL-D-16 选项 A 的最小落地）：
//   - 「存」需要新原语：go2sky v1.5.0 的 `Span` 接口**不暴露 SpanContext**
//     （只有 CreateExitSpanWithContext 的 injector 能拿到编码后的 sw8）⇒ 必须借
//     一次 exit span 编码，故封装成 EncodeSW8；
//   - 「恢复」**不需要**新原语：既有 `CreateEntrySpan(ctx, op, extractor)` 就是
//     "从 sw8 重建父 trace"（NewServerTracingInterceptor / Kafka consumer 一直在用）。
package grpcinterceptor

import (
	"context"
	"reflect"
	"testing"

	"github.com/SkyAPM/go2sky"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTracerInterface_HasEncodeSW8 契约：Tracer 接口必须暴露 EncodeSW8。
func TestTracerInterface_HasEncodeSW8(t *testing.T) {
	it := reflect.TypeOf((*Tracer)(nil)).Elem()
	_, ok := it.MethodByName("EncodeSW8")
	assert.True(t, ok,
		"Tracer 接口缺 EncodeSW8 —— CL-03 需要它把「当前 ctx 的 sw8」取出存进 outbox 行（CL-D-16=A）")
}

// newRealTracer 造一个**带 reporter** 的 go2sky tracer —— 这样 initFlag=1，
// span 与 sw8 编码才是真实路径（不是 noop 降级）。
//
// **为什么必须有 reporter**（2026-10-16 实测，非推断）：go2sky 只在 `reporter != nil`
// 时把 `initFlag` 置 1（`trace.go:78-88`），而 `CreateExitSpanWithContext` 开头
// `if s, nCtx = t.createNoop(ctx); s != nil { return }`（`trace.go:159`）——
// **initFlag==0 时直接返回 NoopSpan 且不调 injector** ⇒ 无 reporter 的 tracer
// 永远编码不出 sw8（本包既有的 StartEntry 测试也因此只走 noop 分支）。
// 生产侧有真 gRPC reporter（BootstrapSkyWalkingTracerRetry）⇒ 编码路径是真的。
//
// reporter 复用同包既有的 `fakeReporter`（client_sw8_bufconn_test.go:25，
// 其注释记录的正是同一个 initFlag 现象）。
func newRealTracer(t *testing.T, name string) *Go2SkyTracer {
	t.Helper()
	realTr, err := go2sky.NewTracer(name, go2sky.WithReporter(fakeReporter{}))
	require.NoError(t, err, "go2sky.NewTracer(带 reporter) 应可构造")
	tr := NewGo2SkyTracer(realTr)
	require.NotNil(t, tr)
	return tr
}

// TestGo2SkyTracer_EncodeSW8_ReturnsWellFormedHeader 基本形态：8 段、采样位 0/1、traceId 可解。
func TestGo2SkyTracer_EncodeSW8_ReturnsWellFormedHeader(t *testing.T) {
	tr := newRealTracer(t, "sw8-encode-shape")

	ctx, sp := tr.StartEntry(context.Background(), "GET /api/v1/chat")
	require.NotNil(t, sp)
	defer sp.EndSpan(nil)

	enc, err := tr.EncodeSW8(ctx, "outbox-enqueue")
	require.NoError(t, err)
	require.NotEmpty(t, enc, "有 span 的 ctx 必须能编码出 sw8")
	assert.NotEmpty(t, TraceIDFromSW8(enc), "编码结果须是能被解析的 sw8（traceId 可解）")
}

// TestGo2SkyTracer_EncodeSW8_NoTracerOrNoSpan_DegradesEmpty 降级：无 tracer / ctx 无 span 时
// 返回空串且不报错（SkyWalking 关掉时不得影响业务写库）。
func TestGo2SkyTracer_EncodeSW8_NoTracerOrNoSpan_DegradesEmpty(t *testing.T) {
	var nilTr *Go2SkyTracer
	sw8, err := nilTr.EncodeSW8(context.Background(), "op")
	assert.NoError(t, err, "nil tracer 不得报错（降级语义）")
	assert.Empty(t, sw8, "nil tracer 必须返回空串")

	// 无 tracer（未启用 SkyWalking）等价路径
	tr := NewGo2SkyTracer(nil)
	if tr != nil { // NewGo2SkyTracer(nil) 返回 nil，保留分支以防实现变化
		sw8, err = tr.EncodeSW8(context.Background(), "op")
		assert.NoError(t, err)
		assert.Empty(t, sw8)
	}
}

// TestSW8RoundTrip_EncodeThenRestore_SameTraceID ★ 本阶段核心断言 ★
//
// 模拟 CL-03 的真实链路：
//   ① 请求 ctx（有入口 span）→ EncodeSW8 → 存进"outbox 行"
//   ② relay 在**后台根 ctx** 上，用存下的 sw8 恢复父 ctx（CreateEntrySpan + extractor）
//   ③ 再编码一次 → trace_id 必须与 ① 相同 ⇒ 两段在同一条 trace 上（断点已接上）
//
// 反例（修复前）：relay 直接用后台根 ctx 发布 ⇒ ③ 的 trace_id 与 ① 不同（两条独立 trace）。
func TestSW8RoundTrip_EncodeThenRestore_SameTraceID(t *testing.T) {
	tr := newRealTracer(t, "sw8-roundtrip")

	// ① 请求侧：有入口 span 的 ctx
	reqCtx, entrySpan := tr.StartEntry(context.Background(), "POST /api/v1/messages")
	require.NotNil(t, entrySpan)
	defer entrySpan.EndSpan(nil)

	storedSW8, err := tr.EncodeSW8(reqCtx, "outbox-enqueue")
	require.NoError(t, err)
	require.NotEmpty(t, storedSW8)
	wantTraceID := TraceIDFromSW8(storedSW8)
	require.NotEmpty(t, wantTraceID)

	// ② relay 侧：后台根 ctx + 用存储的 sw8 恢复父 ctx（这正是 Phase 3 要落地的写法）
	relayRoot := context.Background()
	relayCtx, relaySpan, err := tr.CreateEntrySpan(relayRoot, "outbox-relay",
		func(key string) (string, error) {
			if key == SW8HeaderName {
				return storedSW8, nil
			}
			return "", nil
		})
	require.NoError(t, err)
	require.NotNil(t, relaySpan)
	defer relaySpan.EndSpan(nil)

	// ③ 发布侧：再编码一次，trace_id 必须一致
	publishSW8, err := tr.EncodeSW8(relayCtx, "kafka-publish")
	require.NoError(t, err)
	require.NotEmpty(t, publishSW8)

	assert.Equal(t, wantTraceID, TraceIDFromSW8(publishSW8),
		"relay 恢复父 ctx 后 trace_id 必须与请求侧一致 —— 否则 trace 仍断在 outbox→relay")

	// 反向对照：不恢复父 ctx（仍用后台根 ctx）时 trace_id 必然不同 —— 证明本断言有区分力
	brokenSW8, err := tr.EncodeSW8(relayRoot, "kafka-publish")
	require.NoError(t, err)
	require.NotEmpty(t, brokenSW8)
	assert.NotEqual(t, wantTraceID, TraceIDFromSW8(brokenSW8),
		"不恢复父 ctx 时应是另一条 trace（本断言用于防止测试变成恒真）")
}
