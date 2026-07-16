// Package grpcinterceptor 的 retry 测试

package grpcinterceptor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockInvoker 模拟 invoker 行为
//   - counter: 统计被调用次数
//   - results: 每次调用返回的 error（按顺序消费）
//   - delays:  每次调用前 sleep 多久（模拟服务端处理耗时）
type mockInvoker struct {
	counter int32
	results []error
	delays  []time.Duration
}

func (m *mockInvoker) invoke(_ context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
	idx := atomic.AddInt32(&m.counter, 1) - 1
	if int(idx) < len(m.delays) && m.delays[idx] > 0 {
		time.Sleep(m.delays[idx])
	}
	if int(idx) < len(m.results) {
		return m.results[idx]
	}
	return nil
}

// helper：把 mockInvoker 包成 grpc.UnaryInvoker
func asInvoker(m *mockInvoker) grpc.UnaryInvoker {
	return m.invoke
}

// =====================================================
// Test 1: 成功调用不重试
// =====================================================

func TestClientRetry_NoError_NoRetry(t *testing.T) {
	mock := &mockInvoker{
		results: []error{nil},
	}
	interceptor := ClientRetryInterceptor(DefaultRetryOptions())
	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := atomic.LoadInt32(&mock.counter); got != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", got)
	}
}

// =====================================================
// Test 2: 瞬态错误重试直到成功
// =====================================================

func TestClientRetry_TransientError_RetriesUntilSuccess(t *testing.T) {
	mock := &mockInvoker{
		results: []error{
			status.Error(codes.Unavailable, "try 1"),
			status.Error(codes.DeadlineExceeded, "try 2"),
			nil, // 第 3 次成功
		},
		delays: []time.Duration{0, 0, 0},
	}
	opts := RetryOptions{
		MaxAttempts:       5,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false, // 测试时关掉 jitter 便于断言
		RetryableCodes:    []codes.Code{codes.Unavailable, codes.DeadlineExceeded},
	}
	interceptor := ClientRetryInterceptor(opts)
	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))

	if err != nil {
		t.Fatalf("expected nil error (after retries), got %v", err)
	}
	if got := atomic.LoadInt32(&mock.counter); got != 3 {
		t.Fatalf("expected 3 calls (2 retries), got %d", got)
	}
}

// =====================================================
// Test 3: 不可重试错误立即返回
// =====================================================

func TestClientRetry_NonRetryableError_NoRetry(t *testing.T) {
	mock := &mockInvoker{
		results: []error{
			status.Error(codes.Unauthenticated, "bad token"),
		},
	}
	interceptor := ClientRetryInterceptor(DefaultRetryOptions())
	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&mock.counter); got != 1 {
		t.Fatalf("expected 1 call (no retry for Unauthenticated), got %d", got)
	}
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %s", got)
	}
}

// =====================================================
// Test 4: 达到 MaxAttempts 后返回最后一次错误
// =====================================================

func TestClientRetry_MaxAttemptsReached_ReturnsLastError(t *testing.T) {
	mock := &mockInvoker{
		results: []error{
			status.Error(codes.Unavailable, "try 1"),
			status.Error(codes.Unavailable, "try 2"),
			status.Error(codes.Unavailable, "try 3"),
		},
	}
	opts := RetryOptions{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableCodes:    []codes.Code{codes.Unavailable},
	}
	interceptor := ClientRetryInterceptor(opts)
	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&mock.counter); got != 3 {
		t.Fatalf("expected exactly 3 calls (max attempts), got %d", got)
	}
	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %s", got)
	}
}

// =====================================================
// Test 5: 退避时间指数增长
// =====================================================

func TestClientRetry_BackoffGrowsExponentially(t *testing.T) {
	mock := &mockInvoker{
		results: []error{
			status.Error(codes.Unavailable, "1"),
			status.Error(codes.Unavailable, "2"),
			status.Error(codes.Unavailable, "3"),
		},
	}
	opts := RetryOptions{
		MaxAttempts:       3,
		InitialBackoff:    20 * time.Millisecond,
		MaxBackoff:        500 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableCodes:    []codes.Code{codes.Unavailable},
	}
	interceptor := ClientRetryInterceptor(opts)

	start := time.Now()
	_ = interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))
	elapsed := time.Since(start)

	// 预期 backoff: 20ms + 40ms = 60ms（不含 RPC 时间）
	// 留点余量：>= 50ms 且 < 200ms
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected at least 50ms total backoff, got %v", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("expected at most 200ms total backoff, got %v", elapsed)
	}
}

// =====================================================
// Test 6: ctx 取消立即停止重试
// =====================================================

func TestClientRetry_ContextCancel_StopsRetry(t *testing.T) {
	mock := &mockInvoker{
		results: []error{
			status.Error(codes.Unavailable, "1"),
		},
	}
	opts := RetryOptions{
		MaxAttempts:       10,
		InitialBackoff:    500 * time.Millisecond, // 给 ctx 留时间取消
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableCodes:    []codes.Code{codes.Unavailable},
	}
	interceptor := ClientRetryInterceptor(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := interceptor(ctx, "/test/Method", nil, nil, nil, asInvoker(mock))
	elapsed := time.Since(start)

	// 100ms 内 ctx 超时，应该只调 1 次（第一次失败后 backoff 等待时被取消）
	if elapsed > 400*time.Millisecond {
		t.Fatalf("expected stop within 400ms (ctx cancel), got %v", elapsed)
	}
	if err == nil {
		t.Fatal("expected error (ctx cancel or retry fail)")
	}
	if got := atomic.LoadInt32(&mock.counter); got > 2 {
		t.Fatalf("expected at most 2 calls (ctx cancel), got %d", got)
	}
}

// =====================================================
// Test 7: 默认配置合理
// =====================================================

func TestDefaultRetryOptions(t *testing.T) {
	opts := DefaultRetryOptions()

	if opts.MaxAttempts < 2 {
		t.Fatalf("MaxAttempts must be >= 2, got %d", opts.MaxAttempts)
	}
	if opts.InitialBackoff <= 0 {
		t.Fatalf("InitialBackoff must be > 0, got %v", opts.InitialBackoff)
	}
	if opts.MaxBackoff < opts.InitialBackoff {
		t.Fatalf("MaxBackoff must be >= InitialBackoff, got max=%v init=%v",
			opts.MaxBackoff, opts.InitialBackoff)
	}
	if opts.BackoffMultiplier <= 1.0 {
		t.Fatalf("BackoffMultiplier must be > 1.0, got %v", opts.BackoffMultiplier)
	}

	// 默认重试码必须包含最常见的 Unavailable
	hasUnavailable := false
	for _, c := range opts.RetryableCodes {
		if c == codes.Unavailable {
			hasUnavailable = true
			break
		}
	}
	if !hasUnavailable {
		t.Fatal("default RetryableCodes must include Unavailable")
	}
}

// =====================================================
// Test 8: 自定义 RetryableCodes
// =====================================================

func TestClientRetry_CustomRetryableCodes(t *testing.T) {
	// Internal 是非默认重试码，但用户自定义包含
	mock := &mockInvoker{
		results: []error{
			status.Error(codes.Internal, "1"),
			nil,
		},
	}
	opts := RetryOptions{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableCodes:    []codes.Code{codes.Internal}, // 自定义
	}
	interceptor := ClientRetryInterceptor(opts)
	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))

	if err != nil {
		t.Fatalf("expected nil (custom code Internal retries), got %v", err)
	}
	if got := atomic.LoadInt32(&mock.counter); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

// =====================================================
// Test 9: 非 gRPC status error（如 network err）可重试
// =====================================================

func TestClientRetry_NonStatusError_RetriedAsTransient(t *testing.T) {
	// 没经过 grpc status 包装的 error（TCP RST、conn close 等）
	networkErr := errors.New("connection reset by peer")
	mock := &mockInvoker{
		results: []error{
			networkErr,
			nil,
		},
	}
	opts := RetryOptions{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		MaxBackoff:        5 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableCodes:    []codes.Code{codes.Unavailable},
	}
	interceptor := ClientRetryInterceptor(opts)
	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, asInvoker(mock))

	if err != nil {
		t.Fatalf("expected nil after retry, got %v", err)
	}
	if got := atomic.LoadInt32(&mock.counter); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}
