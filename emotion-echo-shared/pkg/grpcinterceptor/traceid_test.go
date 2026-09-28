// E2E-21 / E2E-F-13：gRPC 侧 trace_id 透传的契约测试。
//
// 背景（2026-09-28 实测）：APISIX 从来不注入 X-Trace-Id（已由 seed.sh 补上），
// 但即便补上，也只走完 HTTP 那一段 —— BFF→downstream 的 gRPC 调用把 ctx 原样传下去，
// 下游 svc 的 NewServerTracingInterceptor 只打 SkyWalking span，从不调 logging.WithTraceID。
// 结果：一次请求在 HTTP 侧有 trace_id、在 gRPC 侧断掉，无法按 ID 查全链路。
//
// 本文件钉住三件事：
//  1. client 侧把 ctx 里的 trace id 写进 outgoing metadata x-trace-id
//  2. server 侧从 incoming metadata 取回并塞进 handler 的 ctx
//  3. client 的调用日志用带 ctx 的 slog 调用（否则 trace_id 永远进不了日志）
//
// 全部用例都带**负向**对照：没有 trace id 时不得伪造一个（伪造会让日志里的
// trace_id 看着有值、实则查不到链路，比恒空更危险）。
package grpcinterceptor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/emotion-echo/shared/pkg/logging"
)

// =====================================================
// 1. client 侧：ctx → outgoing metadata
// =====================================================

// TestClientTraceIDInterceptor_AddsOutgoingMetadata trace id 进 outgoing md
func TestClientTraceIDInterceptor_AddsOutgoingMetadata(t *testing.T) {
	t.Parallel()
	ctx := logging.WithTraceID(context.Background(), "trace-abc123")

	var got metadata.MD
	interceptor := ClientTraceIDInterceptor()
	err := interceptor(ctx, "/emotion_user.v1.UserService/Login", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			got, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
	if err != nil {
		t.Fatalf("invoker err: %v", err)
	}
	if v := got.Get("x-trace-id"); len(v) != 1 || v[0] != "trace-abc123" {
		t.Fatalf("want x-trace-id=trace-abc123, got %v", v)
	}
}

// TestClientTraceIDInterceptor_NoTraceID_DoesNotFabricate 负向：ctx 无 trace 不得伪造
func TestClientTraceIDInterceptor_NoTraceID_DoesNotFabricate(t *testing.T) {
	t.Parallel()
	var got metadata.MD
	interceptor := ClientTraceIDInterceptor()
	err := interceptor(context.Background(), "/svc/X", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			got, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
	if err != nil {
		t.Fatalf("invoker err: %v", err)
	}
	if v := got.Get("x-trace-id"); len(v) != 0 {
		t.Fatalf("ctx 无 trace id 时不得注入 metadata，实际注入 %v", v)
	}
}

// TestClientTraceIDInterceptor_PreservesExistingOutgoingMD 不能把已有的 md 弄丢
//
// 背景：NewClientTracingInterceptor 已经在 outgoing md 里放了 sw8 与 x-user-id。
// 本 interceptor 若直接 md.Set 到 nil map 会 panic；若整体覆盖则前两者丢失。
func TestClientTraceIDInterceptor_PreservesExistingOutgoingMD(t *testing.T) {
	t.Parallel()
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("sw8", "1-TRACE-1-1-1-abc", "x-user-id", "42"))
	ctx = logging.WithTraceID(ctx, "trace-xyz")

	var got metadata.MD
	interceptor := ClientTraceIDInterceptor()
	_ = interceptor(ctx, "/svc/X", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			got, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})

	if v := got.Get("sw8"); len(v) != 1 || v[0] != "1-TRACE-1-1-1-abc" {
		t.Fatalf("sw8 被覆盖/丢失: %v", v)
	}
	if v := got.Get("x-user-id"); len(v) != 1 || v[0] != "42" {
		t.Fatalf("x-user-id 被覆盖/丢失: %v", v)
	}
	if v := got.Get("x-trace-id"); len(v) != 1 || v[0] != "trace-xyz" {
		t.Fatalf("x-trace-id 未注入: %v", v)
	}
}

// =====================================================
// 2. server 侧：incoming metadata → handler ctx
// =====================================================

// TestNewServerTracingInterceptor_PutsTraceIDIntoHandlerCtx 下游 handler 能读到 trace id
func TestNewServerTracingInterceptor_PutsTraceIDIntoHandlerCtx(t *testing.T) {
	t.Parallel()
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-trace-id", "trace-downstream-1"))

	var seen string
	called := false
	// ctxToReturn 留 nil ⇒ mockTracer 原样回传入 ctx（真实 tracer 也是如此）。
	// 若在此处指定一个干净 ctx，等于把 interceptor 刚注入的 trace id 又抹掉，
	// 测的就不是生产行为了。
	_, err := NewServerTracingInterceptor(&mockTracer{
		spanToReturn: &mockSpan{},
	})(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Do"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			called = true
			seen = logging.TraceIDFromCtx(ctx)
			return nil, nil
		})
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if !called {
		t.Fatal("handler 未被调用")
	}
	if seen != "trace-downstream-1" {
		t.Fatalf("want trace-downstream-1 in handler ctx, got %q", seen)
	}
}

// TestNewServerTracingInterceptor_NoHeader_DoesNotFabricate 负向：下游 svc-to-svc 调用无 header 时不得伪造
func TestNewServerTracingInterceptor_NoHeader_DoesNotFabricate(t *testing.T) {
	t.Parallel()
	var seen string
	_, err := NewServerTracingInterceptor(nil)(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Do"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			seen = logging.TraceIDFromCtx(ctx)
			return nil, nil
		})
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if seen != "" {
		t.Fatalf("无 header 时不得伪造 trace id，实际 %q", seen)
	}
}

// =====================================================
// 3. client 调用日志必须带 ctx（否则前面两条全白做）
// =====================================================

// TestClientLoggingInterceptor_LogRecordCarriesTraceID 日志行里真的有 trace_id
//
// 这条最容易漏：ClientLoggingInterceptor 原本用 log.Printf（无 ctx），
// enrichHandler 从 ctx 取 trace_id —— 没有 ctx 就永远取不到，
// 表现为"header 明明注入了、日志里 trace_id 还是空"。
func TestClientLoggingInterceptor_LogRecordCarriesTraceID(t *testing.T) {
	// 刻意不加 t.Parallel：本用例替换全局 slog 默认 logger
	var buf bytes.Buffer
	logging.InitTo(&buf)
	t.Cleanup(func() { logging.Init() })

	ctx := logging.WithTraceID(context.Background(), "trace-log-1")
	interceptor := ClientLoggingInterceptor()
	_ = interceptor(ctx, "/svc/X", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		})

	got := buf.String()
	if !strings.Contains(got, `"trace_id":"trace-log-1"`) {
		t.Fatalf("日志缺 trace_id 字段，实际输出: %s", got)
	}
	// 原有字面量必须保持（client_test.go:170 与 test_jwt_auth_runtime.sh 都依赖它）
	if !strings.Contains(got, "[grpc-client] method=") {
		t.Fatalf("日志格式被破坏，缺 [grpc-client] method= 前缀: %s", got)
	}
}

// TestServerLoggingInterceptor_LogRecordCarriesTraceID 下游服务侧日志带 trace_id
//
// 与 client 侧同型问题：ServerLoggingInterceptor 原用 log.Printf（无 ctx），
// 而 logging.enrichHandler 只从 ctx 取 trace_id ⇒ 下游 svc 的这条日志恒无 trace_id，
// 表现为"BFF 侧查得到、下游查不到"，链路断在最后一跳。
func TestServerLoggingInterceptor_LogRecordCarriesTraceID(t *testing.T) {
	var buf bytes.Buffer
	logging.InitTo(&buf)
	t.Cleanup(func() { logging.Init() })

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-trace-id", "trace-srv-1"))
	// 走完整 server 链里负责取 trace_id 的那个拦截器，再交给 logging
	ctx = ctxWithTraceIDFromMetadata(ctx)

	_, err := ServerLoggingInterceptor()(ctx, nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Do"},
		func(ctx context.Context, req interface{}) (interface{}, error) { return nil, nil })
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"trace_id":"trace-srv-1"`) {
		t.Fatalf("server 日志缺 trace_id 字段，实际输出: %s", got)
	}
	if !strings.Contains(got, "[grpc-server] method=") {
		t.Fatalf("日志格式被破坏: %s", got)
	}
}
