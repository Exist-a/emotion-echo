// Package healthcheck 提供 gRPC 标准 health/v1 协议的封装
//
// TDD 阶段：先写测试，再实现
//
// 标准协议参考：https://github.com/grpc/grpc/blob/master/doc/health-checking.md
// 服务端使用 grpc-go 自带的 google.golang.org/grpc/health 包（已含 grpc_health_v1）
package healthcheck

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bufNetworkListener 启动一个内存 gRPC server（避免端口冲突）
func bufNetworkListener(t *testing.T, srv *grpc.Server) *bufconn.Listener {
	t.Helper()
	lis := bufconn.Listen(1024 * 64)
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("bufconn server stopped: %v", err)
		}
	}()
	return lis
}

// dialBuf 通过 bufconn 拨号（in-memory 客户端）
func dialBuf(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// =====================================================
// Test 1: 默认服务（空 service 名）状态为 Serving
// =====================================================

func TestNewServer_DefaultServingStatus(t *testing.T) {
	srv := NewServer()

	// 新建 server 默认为 Serving（"server liveness"）
	assert.Equal(t, ServingStatusServing, srv.GetServingStatus(""),
		"default empty service should be SERVING (server liveness)")
}

// =====================================================
// Test 2: 注册后可通过 gRPC Check 查询到 SERVING
// =====================================================

func TestServer_CheckReturnsServing(t *testing.T) {
	gs := grpc.NewServer()
	srv := NewServer()
	srv.RegisterWith(gs)
	lis := bufNetworkListener(t, gs)

	conn := dialBuf(t, lis)
	client := healthpb.NewHealthClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
}

// =====================================================
// Test 3: SetServingStatus 后状态变更
// =====================================================

func TestServer_SetServingStatus_NotServing(t *testing.T) {
	srv := NewServer()
	assert.Equal(t, ServingStatusServing, srv.GetServingStatus(""),
		"initial status must be SERVING")

	srv.SetServingStatus("", ServingStatusNotServing)
	assert.Equal(t, ServingStatusNotServing, srv.GetServingStatus(""),
		"status should be NOT_SERVING after SetServingStatus")
}

// =====================================================
// Test 4: 按 service 名独立管理状态（多服务场景）
// =====================================================

func TestServer_PerServiceStatus(t *testing.T) {
	srv := NewServer()

	// 业务服务 A：SERVING
	srv.SetServingStatus("emotion.A", ServingStatusServing)
	// 业务服务 B：未设置（UNKNOWN by default per spec）

	assert.Equal(t, ServingStatusServing, srv.GetServingStatus("emotion.A"))
	assert.Equal(t, ServingStatusUnknown, srv.GetServingStatus("emotion.B"),
		"unset service should be UNKNOWN per gRPC health spec")
}

// =====================================================
// Test 5: Client.Check 远程调用并返回对应状态
// =====================================================

func TestClient_CheckReturnsCurrentStatus(t *testing.T) {
	gs := grpc.NewServer()
	srv := NewServer()
	srv.SetServingStatus("emotion.LLM", ServingStatusServing)
	srv.SetServingStatus("emotion.Broken", ServingStatusNotServing)
	srv.RegisterWith(gs)
	lis := bufNetworkListener(t, gs)

	conn := dialBuf(t, lis)
	cli := NewClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 探活 SERVING 的服务
	st, err := cli.Check(ctx, "emotion.LLM")
	require.NoError(t, err)
	assert.Equal(t, ServingStatusServing, st)

	// 探活 NOT_SERVING 的服务
	st, err = cli.Check(ctx, "emotion.Broken")
	require.NoError(t, err)
	assert.Equal(t, ServingStatusNotServing, st)
}

// =====================================================
// Test 6: Client.Check 查询不存在服务 → 返回 ServiceUnknown
// =====================================================

func TestClient_CheckUnknownService_ReturnsServiceUnknown(t *testing.T) {
	gs := grpc.NewServer()
	srv := NewServer()
	srv.RegisterWith(gs)
	lis := bufNetworkListener(t, gs)

	conn := dialBuf(t, lis)
	cli := NewClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	st, err := cli.Check(ctx, "not.registered.Service")
	// 规范：未注册 service 应返回 NOT_FOUND 错误（rpc error）
	// 但 server 端 grpc.health.Server 默认对未注册返回 ServiceUnknown
	require.Error(t, err, "unknown service should return error")
	assert.Equal(t, ServingStatusServiceUnknown, st,
		"unknown service should map to ServiceUnknown")
}

// =====================================================
// Test 7: WaitForReady 阻塞等待服务变 SERVING
// =====================================================

func TestClient_WaitForReady_SucceedsWhenServing(t *testing.T) {
	gs := grpc.NewServer()
	srv := NewServer()
	srv.SetServingStatus("emotion.LateStart", ServingStatusNotServing)
	srv.RegisterWith(gs)
	lis := bufNetworkListener(t, gs)

	conn := dialBuf(t, lis)
	cli := NewClient(conn)

	// 异步：500ms 后服务变 SERVING
	go func() {
		time.Sleep(500 * time.Millisecond)
		srv.SetServingStatus("emotion.LateStart", ServingStatusServing)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	err := cli.WaitForReady(ctx, "emotion.LateStart", 2*time.Second)
	elapsed := time.Since(start)

	require.NoError(t, err, "WaitForReady should succeed once status flips to SERVING")
	assert.GreaterOrEqual(t, elapsed, 400*time.Millisecond,
		"WaitForReady should have blocked ~500ms before succeeding")
	assert.Less(t, elapsed, 2*time.Second,
		"WaitForReady should not exceed timeout")
}

// =====================================================
// Test 8: WaitForReady 超时返回错误
// =====================================================

func TestClient_WaitForReady_TimeoutWhenNotServing(t *testing.T) {
	gs := grpc.NewServer()
	srv := NewServer()
	srv.SetServingStatus("emotion.Down", ServingStatusNotServing)
	srv.RegisterWith(gs)
	lis := bufNetworkListener(t, gs)

	conn := dialBuf(t, lis)
	cli := NewClient(conn)

	ctx := context.Background()
	start := time.Now()
	err := cli.WaitForReady(ctx, "emotion.Down", 200*time.Millisecond)
	elapsed := time.Since(start)

	require.Error(t, err, "WaitForReady should fail when service stays NOT_SERVING")
	assert.Less(t, elapsed, 500*time.Millisecond,
		"WaitForReady should not block much longer than timeout")
}
