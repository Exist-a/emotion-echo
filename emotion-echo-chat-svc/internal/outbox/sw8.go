// Package outbox — sw8.go
//
// CL-03 Phase 2（写侧）：把「当前 ctx 所在 trace 的 sw8」取出来，写进 outbox 行的
// `trace_sw8` 列；Phase 3 由 relay 在发布前用它恢复父 ctx（relay.go 的 resumeTraceCtx）。
//
// 为什么需要它（建档实测）：chat-svc 的 relay 跑在后台根 ctx 上
// （main.go 的 `relayCtx = context.WithCancel(context.Background())`）⇒
// kafka_publisher 的 CreateExitSpan 建出的是「根」span，一条消息的 trace 断在
// outbox→relay 处。而**写入 outbox 行的时刻身处请求 ctx**（有 gin 入口 span）
// ⇒ 那时把 sw8 存下来，之后就能接回去。
package outbox

import (
	"context"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/logging"
)

// EncodeTraceSW8 取当前 ctx 所在 trace 的 sw8（CL-03 Phase 2 / CL-D-15=A）。
//
// 读的是 **grpcinterceptor.GlobalTracer()**，不是构造期快照的 tracer ——
// 与 CL-F-14「请求期现读全局」一致：svc 先于 OAP 启动时，tracer 是后台补挂上的，
// 构造期快照会漏掉它（实测：漏掉 ⇒ 该进程整段生命周期零 span）。
//
// 降级（**绝不抛错、绝不影响业务写库**）：
//   - 无 tracer（SkyWalking 关闭）⇒ 返回 ""
//   - 编码失败 ⇒ 返回 "" + 一条 log
//
// 返回 "" 时调用方照常写行，只是该行 `trace_sw8` 为空 ⇒ relay 侧退化为改前行为
// （不建 span，直接发布）。这是**有意接受的边界**：SkyWalking 关闭时本就不该有 trace。
//
// operationName 会出现在 trace 里（如 "outbox-enqueue-message"），用于标识
// 「为把 trace 上下文写进 outbox 行而建的编码 span」。
func EncodeTraceSW8(ctx context.Context, operationName string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	tr := grpcinterceptor.GlobalTracer()
	if tr == nil {
		return ""
	}
	sw8, err := tr.EncodeSW8(ctx, operationName)
	if err != nil {
		logging.PrintfContext(ctx,
			"[outbox] encode trace sw8 failed (row keeps empty trace_sw8): %v", err)
		return ""
	}
	return sw8
}
