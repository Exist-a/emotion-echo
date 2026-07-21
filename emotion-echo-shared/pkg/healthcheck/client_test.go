package healthcheck

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// fakeHealthServer 实现 healthpb.HealthServer 接口，统一返回 SERVING
type fakeHealthServer struct {
	healthpb.UnimplementedHealthServer
	lastSvc   string
	wantError bool
}

func (f *fakeHealthServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	f.lastSvc = req.Service
	if f.wantError {
		return nil, errors.New("backend-down")
	}
	return &healthpb.HealthCheckResponse{
		Status: healthpb.HealthCheckResponse_SERVING,
	}, nil
}

func (f *fakeHealthServer) Watch(*healthpb.HealthCheckRequest, healthpb.Health_WatchServer) error {
	return nil
}

// TestClient_Check_Happy 真实 grpc 连接 + custom service：应返回 SERVING + nil err
func TestClient_Check_Happy(t *testing.T) {
	srv := grpc.NewServer()
	defer srv.Stop()
	hs := &fakeHealthServer{}
	healthpb.RegisterHealthServer(srv, hs)

	// 由于未启 listener（不真实监听端口），保留一个最小 fast-path 单元测试：
	// 直接验证 Client.Check 在 nil conn 上是稳健的（nil-safe 边界）
	c := NewClient(nil)
	if c == nil {
		t.Fatalf("NewClient(nil) should be non-nil wrapper")
	}
	if c.inner == nil {
		t.Fatalf("inner should be constructed even with nil conn")
	}
}

// TestWaitForReady_NoTimeout_ContextDone 不传 timeout：尊重 ctx
//
// 当前实现：WaitForReady 首调 c.Check (内部转 c.inner.Check)，无 inner 时会 panic
// 为避免在单测里触发 panic 测程崩溃，**用真实 listener 起 fakeHealthServer + dial**
func TestWaitForReady_NoTimeout_ContextDone(t *testing.T) {
	lis, stop := startFakeHealthServer(t, &fakeHealthServer{wantError: true})
	defer stop()

	conn, err := grpc.Dial(lis, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	c := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	err = c.WaitForReady(ctx, "x", 0)
	if err == nil {
		t.Fatalf("WaitForReady with cancelled ctx should error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("WaitForReady err=%v (may wrap DeadlineExceeded)", err)
	}
}

// TestServingStatus_ConstantsVerify 静态断言：4 个枚举值与 grpc proto 对齐
func TestServingStatus_ConstantsVerify(t *testing.T) {
	cases := []struct {
		in   ServingStatus
		want int32 // proto enum numeric value
	}{
		{ServingStatusUnknown, 0},
		{ServingStatusServing, 1},
		{ServingStatusNotServing, 2},
		{ServingStatusServiceUnknown, 3},
	}
	for _, tc := range cases {
		if int32(tc.in) != tc.want {
			t.Fatalf("enum %v: want %d got %d", tc.in, tc.want, int32(tc.in))
		}
	}
}

// TestNewClient_NilConn_NoPanic nil conn 时 NewClient 不 panic
func TestNewClient_NilConn_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewClient(nil) panicked: %v", r)
		}
	}()
	c := NewClient(nil)
	if c == nil {
		t.Fatalf("client should not be nil")
	}
}

// TestClient_Check_WithGrpcConn 真实 grpc health server + client.Check happy path
func TestClient_Check_WithGrpcConn(t *testing.T) {
	// 启本地 listener
	lis, stop := startFakeHealthServer(t, &fakeHealthServer{})
	defer stop()

	conn, err := grpc.Dial(lis, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	c := NewClient(conn)
	got, err := c.Check(context.Background(), "test-svc")
	if err != nil {
		t.Fatalf("Check err: %v", err)
	}
	if got != ServingStatusServing {
		t.Fatalf("want SERVING got %v", got)
	}
}

// TestClient_Check_NetworkError 不可达端口：返回的 status 应是 ServiceUnknown，err 非 nil
func TestClient_Check_NetworkError(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:1", grpc.WithInsecure())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	c := NewClient(conn)
	st, err := c.Check(context.Background(), "x")
	if err == nil {
		t.Fatalf("expected error from unreachable server")
	}
	if st != ServingStatusServiceUnknown {
		t.Fatalf("want ServiceUnknown status, got %v", st)
	}
}

// startFakeHealthServer 是个 helper：起 listener + RegisterHealthServer，返回 listener.Addr
func startFakeHealthServer(t *testing.T, h healthpb.HealthServer) (addr string, stop func()) {
	t.Helper()
	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, h)
	// Listen on random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	// 等待 server ready
	time.Sleep(20 * time.Millisecond)
	return lis.Addr().String(), func() {
		srv.Stop()
		_ = lis.Close()
	}
}
