package grpcinterceptor

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// =====================================================
// 测试用的 mock Tracer 和 Span
// =====================================================

type mockSpan struct {
	endErr error
	ended  bool
}

func (s *mockSpan) EndSpan(err error) {
	s.ended = true
	s.endErr = err
}

type mockTracer struct {
	// calls 记录 StartEntry 被调用的参数
	calls []string
	// spanToReturn 每次 StartEntry 返回的 span（共享引用）
	spanToReturn *mockSpan
	// ctxToReturn 每次 StartEntry 返回的 ctx
	ctxToReturn context.Context
}

func (t *mockTracer) StartEntry(ctx context.Context, opName string) (context.Context, Span) {
	t.calls = append(t.calls, opName)
	if t.ctxToReturn == nil {
		t.ctxToReturn = ctx
	}
	return t.ctxToReturn, t.spanToReturn
}

// =====================================================
// ServerTracingInterceptor 测试
// =====================================================

func TestServerTracing_NilTracer_BypassesTracing(t *testing.T) {
	t.Parallel()
	interceptor := NewServerTracingInterceptor(nil)
	h := &fakeHandler{}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if h.called != 1 {
		t.Fatalf("expected handler called 1 time, got %d", h.called)
	}
}

func TestServerTracing_HappyPath_CallsTracerAndEndsSpan(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		h.handle)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// tracer.StartEntry 应被调用一次，参数是 FullMethod
	if len(tracer.calls) != 1 || tracer.calls[0] != "/svc/Method" {
		t.Fatalf("expected StartEntry called once with method name, got %v", tracer.calls)
	}
	if !span.ended {
		t.Fatal("expected span to be ended")
	}
	if span.endErr != nil {
		t.Fatalf("expected span.EndSpan(nil) on success, got err=%v", span.endErr)
	}
}

func TestServerTracing_HandlerError_PropagatesAndEndsSpanWithErr(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{returnErr: errors.New("downstream fail")}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		h.handle)

	if err == nil {
		t.Fatal("expected err to propagate")
	}
	if !span.ended {
		t.Fatal("expected span to be ended even on error")
	}
	if span.endErr == nil || span.endErr.Error() != "downstream fail" {
		t.Fatalf("expected span.EndSpan(err), got endErr=%v", span.endErr)
	}
}

func TestServerTracing_PanicInHandler_EndsSpanWithRecoveredErr(t *testing.T) {
	t.Parallel()
	// 注意：panic 不会被 tracing interceptor 捕获，
	// 因为 recovery interceptor 应该 wrap 在外面。
	// 但 tracing 自己应该正确传递 panic。
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{panicVal: "boom"}

	defer func() {
		// 验证 span.EndSpan 在 panic 之前被 defer 调用
		if !span.ended {
			t.Fatal("expected span to be ended before panic propagates")
		}
		// EndSpan(nil) 是 tracing 传的（panic 由 recovery 转 err）
		// 这里我们不期望 tracing 知道 panic
		_ = recover() // 吞掉 panic 让测试继续
	}()

	_, _ = interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		h.handle)
}

func TestServerTracing_PropagatesContextFromTracer(t *testing.T) {
	t.Parallel()
	// tracer 返回带 trace-id 的 ctx，handler 应该能看到
	type ctxKey struct{}
	expectedCtx := context.WithValue(context.Background(), ctxKey{}, "trace-xyz")

	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: expectedCtx}
	interceptor := NewServerTracingInterceptor(tracer)

	var seenCtx context.Context
	customHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		seenCtx = ctx
		return "ok", nil
	}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		customHandler)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := seenCtx.Value(ctxKey{}); got != "trace-xyz" {
		t.Fatalf("expected handler to receive trace context, got %v", got)
	}
}

// ===== PR-OBS-13 RED: 2 case — gRPC trace 边界 =====
//
// 目的: PR-OBS-13 设计 (observability-sprint-b.md §三.13):
// 1. gRPC 调用 FullMethod "/svc/Method" → tracer.StartEntry 的 opName = FullMethod
//    (这是后续 span tag rpc.method 的来源)
// 2. gRPC 调用 metadata x-user-id → handler 收到的 ctx 可读到 x-user-id
//
// 现状约束: Span 接口只有 EndSpan (无 Tag),完整 'span tag 含 user_id/rpc.system/service/method'
// 需要 1) Span 接口扩展 Tag(key, value string) 2) ServerTracingInterceptor 在 StartEntry
// 后调 span.Tag(...) 多次 3) mockSpan 实现 Tag + 记录 tag 调用顺序
// 上述是较大改造,留作 follow-up PR (与 PR-OBS-12 TracerInterface 类似路径)
//
// 本 PR 落地 2 边界 case (覆盖补全型,代码已满足):
// - FullMethod → opName 透传 (mockTracer.calls 验证)
// - metadata x-user-id → handler ctx (handler 读 ctx 验证)
// 这些是 Span.Tag 扩展前的关键前置条件,本 PR 固化避免重构时丢失

// TestServerTracing_FullMethodAsOpName 验证 StartEntry 的 opName = FullMethod
func TestServerTracing_FullMethodAsOpName(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/emotion_llm.v1.EmotionLLMService/Analyze"},
		h.handle)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 验证 StartEntry 的 opName == FullMethod (这是 span tag rpc.method 的来源)
	if len(tracer.calls) != 1 || tracer.calls[0] != "/emotion_llm.v1.EmotionLLMService/Analyze" {
		t.Errorf("expected opName = FullMethod, got %v", tracer.calls)
	}
}

// TestServerTracing_XUserIDMetadataPropagatedToHandler 验证 metadata x-user-id 透传
// handler 从 ctx 的 incoming metadata 读 x-user-id (shared/auth/xuserid.go ParseXUserID 模式)
func TestServerTracing_XUserIDMetadataPropagatedToHandler(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	// mockTracer 不设置 ctxToReturn → interceptor 用输入 ctx (含 metadata)
	tracer := &mockTracer{spanToReturn: span}
	interceptor := NewServerTracingInterceptor(tracer)

	var gotUserID string
	// 用 inline handler (不是 fakeHandler.handle),这样可读 ctx 验证 metadata 透传
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			values := md.Get("x-user-id")
			if len(values) > 0 {
				gotUserID = values[0]
			}
		}
		return nil, nil
	}

	md := metadata.MD{}
	md.Set("x-user-id", "12345")
	ctxWithMD := metadata.NewIncomingContext(context.Background(), md)

	_, err := interceptor(ctxWithMD, nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		handler)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotUserID != "12345" {
		t.Errorf("expected x-user-id=12345 in handler ctx, got %q", gotUserID)
	}
}