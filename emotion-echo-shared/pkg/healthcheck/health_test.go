// healthcheck 的 **client 侧**行为测试
//
// 关键保真度约定（E2E-23 F-163，2026-09-30）：
//
//	server 侧一律使用 **上游 google.golang.org/grpc/health**，
//	因为 5 个 Go 服务的 internal/grpcserver/server.go 就是这么做的。
//	本包原先那个 server 侧包装（Server/NewServer/RegisterWith/…）因生产零调用方已删除。
//	⇒ 本文件**必须**对着上游 health server 测 Client，
//	否则就是"client 对着一个生产不存在的替身实现测"，无任何保真度。
//	守卫：scripts/test_healthcheck_no_dead_server.sh 第 5 条断言本文件 import 了上游包。
//
// 标准协议参考：https://github.com/grpc/grpc/blob/master/doc/health-checking.md
package healthcheck

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
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
// Test 1: Client.Check 远程调用并返回对应状态
// =====================================================

func TestClient_CheckReturnsCurrentStatus(t *testing.T) {
	gs := grpc.NewServer()
	srv := health.NewServer()
	srv.SetServingStatus("emotion.LLM", healthpb.HealthCheckResponse_SERVING)
	srv.SetServingStatus("emotion.Broken", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(gs, srv)
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
// Test 2: Client.Check 查询不存在服务 → 返回 ServiceUnknown
// =====================================================

func TestClient_CheckUnknownService_ReturnsServiceUnknown(t *testing.T) {
	gs := grpc.NewServer()
	srv := health.NewServer()
	healthpb.RegisterHealthServer(gs, srv)
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
// Test 3: WaitForReady 阻塞等待服务变 SERVING
// =====================================================

func TestClient_WaitForReady_SucceedsWhenServing(t *testing.T) {
	gs := grpc.NewServer()
	srv := health.NewServer()
	srv.SetServingStatus("emotion.LateStart", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(gs, srv)
	lis := bufNetworkListener(t, gs)

	conn := dialBuf(t, lis)
	cli := NewClient(conn)

	// 异步：500ms 后服务变 SERVING
	go func() {
		time.Sleep(500 * time.Millisecond)
		srv.SetServingStatus("emotion.LateStart", healthpb.HealthCheckResponse_SERVING)
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
// Test 4: WaitForReady 超时返回错误
// =====================================================

func TestClient_WaitForReady_TimeoutWhenNotServing(t *testing.T) {
	gs := grpc.NewServer()
	srv := health.NewServer()
	srv.SetServingStatus("emotion.Down", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(gs, srv)
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
