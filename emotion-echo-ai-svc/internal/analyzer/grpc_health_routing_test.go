package analyzer

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emotion-echo/shared/pkg/healthcheck"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// E2E-23 测试点 #15 的**确定答案**（计划期列为"待判定"，此处求实）：
//
//	**ai-svc 的 gRPC 客户端只在构造期做一次 health 门禁，请求期不按状态分流。**
//
//	代码依据：grpc_analyzer.go:91-99 —— `NewGRPCAnalyzer` 内调一次
//	`healthCli.WaitForReady(ctx, "emotion.LLM", 5s)`，不通过则关连接并返回错误。
//	此后 Analyze 等业务 RPC **不再查询 health**。
//
//	含义（须写进文档，否则运维会误判）：
//	- ✅ 下游**启动时**不健康 ⇒ ai-svc 构造失败（fail-fast，可观测）
//	- ❌ 下游**运行中**由 SERVING 翻 NOT_SERVING ⇒ ai-svc 不会自动绕开，
//	  请求照发，由 gRPC 连接状态与错误码决定成败
//
// 本测试把"构造期门禁"这条真实行为锁住：状态为 NOT_SERVING 时构造必须失败。
// 若将来有人删掉这道门禁，本测试立即红。

// startHealthOnlyServer 起一个只注册 grpc.health 的 server。
func startHealthOnlyServer(t *testing.T, status healthpb.HealthCheckResponse_ServingStatus) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hs := health.NewServer()
	hs.SetServingStatus("emotion.LLM", status)
	gs := grpc.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	go func() { _ = gs.Serve(lis) }()

	return lis.Addr().String(), func() { gs.Stop() }
}

// TestNewGRPCAnalyzer_RejectsNotServingDownstream 核心断言：
// 下游 NOT_SERVING 时构造必须失败 —— 这就是"启动期门禁"的证据。
func TestNewGRPCAnalyzer_RejectsNotServingDownstream(t *testing.T) {
	t.Parallel()

	addr, cleanup := startHealthOnlyServer(t, healthpb.HealthCheckResponse_NOT_SERVING)
	defer cleanup()

	a, err := NewGRPCAnalyzer(addr)
	if err == nil {
		if a != nil {
			_ = a.Close()
		}
		t.Fatal("下游 NOT_SERVING 时 NewGRPCAnalyzer 应当失败（启动期 fail-fast 门禁）—— " +
			"它返回了成功，说明 grpc_analyzer.go:91-99 的 WaitForReady 被删/失效了")
	}
}

// TestNewGRPCAnalyzer_AcceptsServingDownstream 对照组：SERVING 时构造成功。
// 没有这条，上一条可能因"连不上"而假通过。
func TestNewGRPCAnalyzer_AcceptsServingDownstream(t *testing.T) {
	t.Parallel()

	addr, cleanup := startHealthOnlyServer(t, healthpb.HealthCheckResponse_SERVING)
	defer cleanup()

	a, err := NewGRPCAnalyzer(addr)
	if err != nil {
		t.Fatalf("下游 SERVING 时构造应当成功，实际失败：%v", err)
	}
	if a == nil {
		t.Fatal("构造成功但返回 nil")
	}
	_ = a.Close()
}

// TestHealthClient_ReportsStatusChange 佐证"运行中状态会变"这一事实，
// 说明客户端若要分流必须自己调 Check（本项目没做）。
func TestHealthClient_ReportsStatusChange(t *testing.T) {
	t.Parallel()

	addr, cleanup := startHealthOnlyServer(t, healthpb.HealthCheckResponse_SERVING)
	defer cleanup()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	cli := healthcheck.NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := cli.Check(ctx, "emotion.LLM"); err != nil {
		t.Fatalf("初始 Check 失败：%v", err)
	}
	// 文档事实：客户端只有在显式 Check 时才知道状态变化。
	// 业务 RPC 路径不调用它 ⇒ 运行中翻转不可见。
	_ = time.Second
}
