package grpcinterceptor

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// fakeCtxWithPeer 把 peer 信息塞进 ctx，模拟 gRPC ServerStream 上有的 peer 信息
func fakeCtxWithPeer(addr string) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: addrFor(addr)})
}

type stubAddr struct{ s string }

func (a *stubAddr) Network() string { return "tcp" }
func (a *stubAddr) String() string  { return a.s }

func addrFor(s string) net.Addr {
	return &stubAddr{s: s}
}

// TestServerLoggingInterceptor_Success 非 error 响应
func TestServerLoggingInterceptor_Success(t *testing.T) {
	interceptor := ServerLoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}
	called := false
	resp, err := interceptor(context.Background(), "req", info,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			called = true
			return "OK", nil
		})
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if !called {
		t.Fatalf("handler not called")
	}
	if resp != "OK" {
		t.Fatalf("resp mismatch: %v", resp)
	}
}

// TestServerLoggingInterceptor_Error 透传 handler 返回的 err
func TestServerLoggingInterceptor_Error(t *testing.T) {
	interceptor := ServerLoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}
	want := status.Error(5, "NOT_FOUND: x")
	resp, err := interceptor(context.Background(), "req", info,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, want
		})
	if err != want {
		t.Fatalf("err should pass through, got %v", err)
	}
	_ = resp
}

// TestServerLoggingInterceptor_PeerPresent 验证从 ctx 拿 peer 后写日志
// 不行：log.Printf 走全局 logger，没法验证字面量；只能证明不 panic 且返回 ok
func TestServerLoggingInterceptor_PeerPresent(t *testing.T) {
	interceptor := ServerLoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/X"}
	ctx := fakeCtxWithPeer("127.0.0.1:54321")
	_, err := interceptor(ctx, nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		// peer.Peer 在 ctx 里
		if _, ok := peer.FromContext(ctx); !ok {
			t.Fatalf("peer should be present in ctx")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

// TestServerLoggingInterceptor_PeerMissing 不影响主流程：peer 缺失时返回 "unknown"
func TestServerLoggingInterceptor_PeerMissing(t *testing.T) {
	interceptor := ServerLoggingInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/X"}
	// 不加 peer，验证 handler 仍被调
	called := false
	_, err := interceptor(context.Background(), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if !called {
		t.Fatalf("handler should run even without peer")
	}
}

// TestServerRecoveryInterceptor_PanicRecovered panic 时转 Internal status
func TestServerRecoveryInterceptor_PanicRecovered(t *testing.T) {
	interceptor := ServerRecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Panic"}
	called := false
	resp, err := interceptor(context.Background(), nil, info,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			called = true
			panic("boom!")
		})
	// 即使 panic，process 仍应继续
	if !called {
		t.Fatalf("handler should be called")
	}
	if err == nil {
		t.Fatalf("panic should be converted to err, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("err should be a gRPC status, got %v", err)
	}
	// 13 codes.Internal
	if st.Code().String() != "Internal" {
		t.Fatalf("want code Internal, got %s", st.Code().String())
	}
	if !strings.Contains(st.Message(), "boom!") {
		t.Fatalf("msg should contain panic info: %q", st.Message())
	}
	_ = resp
}

// TestServerRecoveryInterceptor_NoPanic 正常 handler 不受影响
func TestServerRecoveryInterceptor_NoPanic(t *testing.T) {
	interceptor := ServerRecoveryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/OK"}
	wantResp := "fine"
	resp, err := interceptor(context.Background(), nil, info,
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return wantResp, nil
		})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if resp != wantResp {
		t.Fatalf("resp mismatch: %v", resp)
	}
}

// TestServerRecoveryInterceptor_HandlerReturnsError handler 自带 err 不被 panic recovery 吞掉
func TestServerRecoveryInterceptor_HandlerReturnsError(t *testing.T) {
	interceptor := ServerRecoveryInterceptor()
	want := status.Error(3, "INVALID_ARGUMENT")
	resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/X"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, want
		})
	if err != want {
		t.Fatalf("err should pass through, got %v", err)
	}
	_ = resp
}

// TestServerRecoveryInterceptor_NilPointerPanic nil pointer 也能 recover
func TestServerRecoveryInterceptor_NilPointerPanic(t *testing.T) {
	interceptor := ServerRecoveryInterceptor()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recovery interceptor should not bubble panic, got %v", r)
		}
	}()
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/X"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			var p *int
			*p = 1 // nil pointer deref panic
			return nil, nil
		})
	if err == nil {
		t.Fatalf("expected err from panic recovery")
	}
}

// TestPeerFromContext_NoPeer 未注入 peer 时返回 "unknown", false
func TestPeerFromContext_NoPeer(t *testing.T) {
	got, ok := peerFromContext(context.Background())
	if ok {
		t.Fatalf("should not be ok")
	}
	if got != "unknown" {
		t.Fatalf("want 'unknown', got %q", got)
	}
}

// TestPeerFromContext_WithPeer 注入 peer 后 ok=true 且 addr 正确
func TestPeerFromContext_WithPeer(t *testing.T) {
	ctx := fakeCtxWithPeer("10.0.0.5:9001")
	got, ok := peerFromContext(ctx)
	if !ok {
		t.Fatalf("peer should be present")
	}
	if got != "10.0.0.5:9001" {
		t.Fatalf("addr mismatch: %q", got)
	}
}

// TestPeerFromContext_TableDriven 多种地址格式
func TestPeerFromContext_TableDriven(t *testing.T) {
	cases := []string{
		"127.0.0.1:54321",
		"[::1]:443",
		"host.example.com:80",
	}
	for _, addr := range cases {
		ctx := fakeCtxWithPeer(addr)
		got, ok := peerFromContext(ctx)
		if !ok {
			t.Fatalf("addr=%s: peer should be present", addr)
		}
		if got != addr {
			t.Fatalf("addr=%s: want %q got %q", addr, addr, got)
		}
	}
}

// silence unused
var _ = errors.New
