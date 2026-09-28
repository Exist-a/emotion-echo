// E2E-21 收尾 / E2E-F-146：带 ctx 的日志 helper 契约测试。
//
// 背景：enrichHandler 从 **ctx** 取 trace_id 再注入日志字段（logging.go:126-139），
// 而全仓 160 处 `log.Printf` 不带 ctx ⇒ 即便 traceId 链路已打通，
// handler 内部逐条日志仍查不到 trace_id。本组测试钉住"带 ctx 的调用方式"，
// 让调用点有理由迁移、并防止新代码又用回无 ctx 的形式。
package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestPrintfContext_InjectsTraceIDFromCtx 迁移后的写法必须真的带出 trace_id
func TestPrintfContext_InjectsTraceIDFromCtx(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	t.Cleanup(func() { Init() })

	ctx := WithTraceID(context.Background(), "trace-helper-1")
	PrintfContext(ctx, "[demo] user=%s n=%d", "u1", 3)

	got := buf.String()
	if !strings.Contains(got, `"trace_id":"trace-helper-1"`) {
		t.Fatalf("PrintfContext 未注入 trace_id: %s", got)
	}
	if !strings.Contains(got, `"msg":"[demo] user=u1 n=3"`) {
		t.Fatalf("PrintfContext 格式化结果不对: %s", got)
	}
}

// TestPrintfContext_NoCtx_DoesNotPanic 无 ctx 时也必须能用（后台任务/启动代码）
func TestPrintfContext_NoCtx_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	t.Cleanup(func() { Init() })

	PrintfContext(context.Background(), "[demo] no ctx here") //nolint:staticcheck // 故意传无 trace 的 ctx
	if !strings.Contains(buf.String(), "no ctx here") {
		t.Fatalf("无 ctx 时应照常输出: %s", buf.String())
	}
}

// TestErrorContext_KeepsErrAsStructuredField err 应作为结构化字段而非拼进 msg
func TestErrorContext_KeepsErrAsStructuredField(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	t.Cleanup(func() { Init() })

	ctx := WithTraceID(context.Background(), "trace-helper-2")
	ErrorContext(ctx, "[demo] persist failed", "err", "boom")

	got := buf.String()
	if !strings.Contains(got, `"err":"boom"`) {
		t.Fatalf("err 未作为结构化字段: %s", got)
	}
	if !strings.Contains(got, `"level":"ERROR"`) {
		t.Fatalf("level 应为 ERROR: %s", got)
	}
	if !strings.Contains(got, `"trace_id":"trace-helper-2"`) {
		t.Fatalf("ErrorContext 未注入 trace_id: %s", got)
	}
}
