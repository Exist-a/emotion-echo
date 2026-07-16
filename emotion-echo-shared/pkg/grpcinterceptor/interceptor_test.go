package grpcinterceptor

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
)

// fakeHandler 模拟一个 RPC handler（interceptor 调用）
type fakeHandler struct {
	called   int
	returnErr error
	panicVal  interface{}
}

func (h *fakeHandler) handle(ctx context.Context, req interface{}) (interface{}, error) {
	h.called++
	if h.panicVal != nil {
		panic(h.panicVal)
	}
	return "ok-response", h.returnErr
}

// =====================================================
// ServerLoggingInterceptor 测试
// =====================================================

func TestServerLogging_PassesThroughSuccess(t *testing.T) {
	t.Parallel()
	h := &fakeHandler{}
	interceptor := ServerLoggingInterceptor()

	resp, err := interceptor(
		context.Background(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok-response" {
		t.Fatalf("expected resp='ok-response', got=%v", resp)
	}
	if h.called != 1 {
		t.Fatalf("expected handler called 1 time, got %d", h.called)
	}
}

func TestServerLogging_PropagatesError(t *testing.T) {
	t.Parallel()
	h := &fakeHandler{returnErr: errors.New("downstream error")}
	interceptor := ServerLoggingInterceptor()

	_, err := interceptor(
		context.Background(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle,
	)
	if err == nil || err.Error() != "downstream error" {
		t.Fatalf("expected downstream error, got: %v", err)
	}
}

// =====================================================
// ServerRecoveryInterceptor 测试
// =====================================================

func TestServerRecovery_PanicRecovered(t *testing.T) {
	t.Parallel()
	h := &fakeHandler{panicVal: "boom!"}
	interceptor := ServerRecoveryInterceptor()

	// 测试不应该让测试进程崩溃
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic leaked through interceptor: %v", r)
		}
	}()

	_, err := interceptor(
		context.Background(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle,
	)
	if err == nil {
		t.Fatal("expected error after panic recovery")
	}
	// gRPC status code 13 = Internal
	if !contains(err.Error(), "internal error") {
		t.Fatalf("expected 'internal error' in err, got: %v", err)
	}
}

func TestServerRecovery_NormalCallNotAffected(t *testing.T) {
	t.Parallel()
	h := &fakeHandler{}
	interceptor := ServerRecoveryInterceptor()

	resp, err := interceptor(
		context.Background(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok-response" {
		t.Fatalf("expected resp='ok-response', got=%v", resp)
	}
}

// =====================================================
// ClientTimeoutInterceptor 测试
// =====================================================

func TestClientTimeout_AddsTimeoutWhenNoDeadline(t *testing.T) {
	t.Parallel()
	interceptor := ClientTimeoutInterceptor(50 * time.Millisecond)

	called := false
	fakeInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		called = true
		// 检查 ctx 现在有 deadline
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected ctx to have deadline after interceptor")
		}
		// deadline 应该在 50ms 之后
		remaining := time.Until(deadline)
		if remaining < 0 || remaining > 100*time.Millisecond {
			t.Fatalf("deadline not in expected range, remaining=%v", remaining)
		}
		return nil
	}

	// nil cc OK 因为 invoker 不调用它
	_ = interceptor(
		context.Background(),
		"/Test/Method",
		nil, nil, nil,
		fakeInvoker,
	)
	if !called {
		t.Fatal("expected invoker to be called")
	}
}

func TestClientTimeout_PreservesExistingDeadline(t *testing.T) {
	t.Parallel()
	interceptor := ClientTimeoutInterceptor(50 * time.Millisecond)

	// 已有 deadline 的 ctx
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	var observedRemaining time.Duration
	fakeInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		deadline, _ := ctx.Deadline()
		observedRemaining = time.Until(deadline)
		return nil
	}

	_ = interceptor(ctx, "/Test/Method", nil, nil, nil, fakeInvoker)

	// 应该保留 1h deadline，不被 50ms 覆盖
	if observedRemaining < 30*time.Minute {
		t.Fatalf("expected original deadline preserved, got remaining=%v", observedRemaining)
	}
}

// =====================================================
// 工具函数
// =====================================================

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}