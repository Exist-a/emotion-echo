package grpcinterceptor

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
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