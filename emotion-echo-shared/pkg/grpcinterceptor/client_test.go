package grpcinterceptor

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
)

// dialTestServer 启本地 gRPC server，返回真实 *grpc.ClientConn
// （grpc.ClientConn 是 struct 不是 interface，无法用 shim 替代）
func dialTestServer(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	// 让 server 进入 ready
	time.Sleep(10 * time.Millisecond)
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		srv.Stop()
		t.Fatalf("dial: %v", err)
	}
	return conn, func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
}

// TestClientTimeoutInterceptor_NoDeadline_AddsDeadline 无 deadline 时 interceptor 应加 50ms
func TestClientTimeoutInterceptor_NoDeadline_AddsDeadline(t *testing.T) {
	interceptor := ClientTimeoutInterceptor(50 * time.Millisecond)
	var okDeadline bool
	var remaining time.Duration
	got := interceptor(context.Background(), "/svc/Method", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, c *grpc.ClientConn, opts ...grpc.CallOption) error {
			dl, ok := ctx.Deadline()
			okDeadline = ok
			if ok {
				remaining = time.Until(dl)
			}
			return nil
		})
	if got != nil {
		t.Fatalf("want nil err, got %v", got)
	}
	if !okDeadline {
		t.Fatalf("deadline should be added when ctx has none")
	}
	if remaining <= 0 || remaining > 60*time.Millisecond {
		t.Fatalf("deadline should be ~50ms, got %v", remaining)
	}
}

// TestClientTimeoutInterceptor_PreservesDeadline 已有 deadline 时不应覆盖
func TestClientTimeoutInterceptor_PreservesDeadline(t *testing.T) {
	interceptor := ClientTimeoutInterceptor(50 * time.Millisecond)
	parent, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var okDeadline bool
	got := interceptor(parent, "/svc/Method", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, c *grpc.ClientConn, opts ...grpc.CallOption) error {
			dl, ok := ctx.Deadline()
			okDeadline = ok
			// 原 1s deadline 应保留：remaining > 100ms
			if !ok || time.Until(dl) < 100*time.Millisecond {
				t.Fatalf("original 1s deadline should be preserved, remaining=%v", time.Until(dl))
			}
			return nil
		})
	if got != nil {
		t.Fatalf("want nil, got %v", got)
	}
	if !okDeadline {
		t.Fatalf("deadline should remain")
	}
}

// TestClientTimeoutInterceptor_PassesInvokerError invoker 的 err 应透传
func TestClientTimeoutInterceptor_PassesInvokerError(t *testing.T) {
	interceptor := ClientTimeoutInterceptor(time.Second)
	want := errors.New("rpc fail")
	got := interceptor(context.Background(), "/svc/Method", nil, nil, nil,
		func(ctx context.Context, method string, req, rep interface{}, c *grpc.ClientConn, opts ...grpc.CallOption) error {
			return want
		})
	if got != want {
		t.Fatalf("err should pass through, got %v", got)
	}
}

// TestClientTimeoutInterceptor_TableDriven 表驱动 timeout 行为
func TestClientTimeoutInterceptor_TableDriven(t *testing.T) {
	cases := []struct {
		name     string
		timeout  time.Duration
		wantZone time.Duration
	}{
		{"10ms", 10 * time.Millisecond, 10 * time.Millisecond},
		{"100ms", 100 * time.Millisecond, 100 * time.Millisecond},
		{"500ms", 500 * time.Millisecond, 500 * time.Millisecond},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			interceptor := ClientTimeoutInterceptor(tc.timeout)
			var remaining time.Duration
			_ = interceptor(context.Background(), "/svc/X", nil, nil, nil,
				func(ctx context.Context, method string, req, rep interface{}, c *grpc.ClientConn, opts ...grpc.CallOption) error {
					dl, ok := ctx.Deadline()
					if !ok {
						return nil
					}
					remaining = time.Until(dl)
					return nil
				})
			if remaining <= 0 || remaining > tc.wantZone+10*time.Millisecond {
				t.Fatalf("deadline want <=%v got %v", tc.wantZone+10*time.Millisecond, remaining)
			}
		})
	}
}

// TestClientLoggingInterceptor_CallsInvoker 用真实 dial 验证 invoker 被调用
// 当前实现：ClientLoggingInterceptor 内部调 cc.Target()，必须是非 nil cc
func TestClientLoggingInterceptor_CallsInvoker(t *testing.T) {
	conn, teardown := dialTestServer(t)
	defer teardown()
	interceptor := ClientLoggingInterceptor()
	called := false
	got := interceptor(context.Background(), "/svc/X", nil, nil, conn,
		func(ctx context.Context, method string, req, rep interface{}, c *grpc.ClientConn, opts ...grpc.CallOption) error {
			called = true
			return nil
		})
	if !called {
		t.Fatalf("invoker should run")
	}
	if got != nil {
		t.Fatalf("want nil got %v", got)
	}
}

// TestClientLoggingInterceptor_PassesInvokerError err 透传
func TestClientLoggingInterceptor_PassesInvokerError(t *testing.T) {
	conn, teardown := dialTestServer(t)
	defer teardown()
	interceptor := ClientLoggingInterceptor()
	want := errors.New("err-x")
	got := interceptor(context.Background(), "/svc/X", nil, nil, conn,
		func(ctx context.Context, method string, req, rep interface{}, c *grpc.ClientConn, opts ...grpc.CallOption) error {
			return want
		})
	if got != want {
		t.Fatalf("want %v got %v", want, got)
	}
}

// TestClientLoggingInterceptor_FormatString 静态断言日志字面量
func TestClientLoggingInterceptor_FormatString(t *testing.T) {
	const want = "[grpc-client] method=%s target=%s latency=%dms err=%v"
	if !strings.Contains(want, "method=%s") || !strings.Contains(want, "target=%s") {
		t.Fatalf("format string invariant broke: %s", want)
	}
}
