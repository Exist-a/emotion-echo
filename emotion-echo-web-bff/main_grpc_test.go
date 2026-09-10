// Package main — main_grpc_test.go
//
// Stage 63 RED: BFF gRPC 接线测试
//
// 背景 bug（Stage 62 §五.4）：PR-3 建好了 3 svc gRPC server + BFF gRPC client 实现，
// 但 main.go buildServiceContext 构造 4 个下游 client 时只传 BaseURL，不传 GRPCConn。
// 下游工厂逻辑：Transport=grpc（默认）+ GRPCConn==nil → 静默 fallback 到 HTTP。
// 结果：生产路径仍全走 HTTP，gRPC 投资等于没上线。
//
// 本测试断言：buildServiceContext 在 GRPCAddr 配置后真的 dial 并传入 GRPCConn，
// 返回的是 gRPC client（而非 HTTP fallback）。
//
// 验证方式：用反射检查返回 client 的具体类型名（*userGRPCClient vs *userHTTPClient）。
// 下游 gRPC 类型未导出，但类型名含 "GRPC" / "HTTP" 可区分。
package main

import (
	"context"
	"net"
	"reflect"
	"testing"

	"emotion-echo-web-bff/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// bufDialer 是 bufconn 的 dial 函数，供测试覆盖 grpcDialer 用。
func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
}

// clientTypeName 返回 client 具体类型的名字（如 "userGRPCClient" / "userHTTPClient"）。
func clientTypeName(client any) string {
	if client == nil {
		return "<nil>"
	}
	return reflect.TypeOf(client).Elem().Name()
}

// isGRPCClient 断言 client 是 gRPC 实现（类型名含 "GRPC"）。
func isGRPCClient(t *testing.T, client any) {
	t.Helper()
	name := clientTypeName(client)
	assert.Contains(t, name, "GRPC", "期望 gRPC client，实际类型: %s", name)
}

// isHTTPClient 断言 client 是 HTTP 实现（类型名含 "HTTP"）。
func isHTTPClient(t *testing.T, client any) {
	t.Helper()
	name := clientTypeName(client)
	assert.Contains(t, name, "HTTP", "期望 HTTP client，实际类型: %s", name)
}

// TestBuildServiceContext_GRPCWiring_AllFourClients 核心 RED 测试：
// 当 4 个下游都配置了 GRPCAddr 且 dial 成功时，buildServiceContext 必须返回 gRPC client。
//
// 修复前：4 个 client 全是 HTTP fallback（GRPCConn 未传入）→ 本测试 FAIL。
// 修复后：4 个 client 全是 gRPC 实现 → PASS。
func TestBuildServiceContext_GRPCWiring_AllFourClients(t *testing.T) {
	// 启动 4 个 bufconn gRPC server
	userLis := bufconn.Listen(1024 * 1024)
	chatLis := bufconn.Listen(1024 * 1024)
	assessmentLis := bufconn.Listen(1024 * 1024)
	analyticsLis := bufconn.Listen(1024 * 1024)
	for _, l := range []*bufconn.Listener{userLis, chatLis, assessmentLis, analyticsLis} {
		srv := grpc.NewServer()
		go func(lis *bufconn.Listener, s *grpc.Server) {
			_ = s.Serve(lis)
		}(l, srv)
		t.Cleanup(func() { srv.Stop(); _ = l.Close() })
	}

	// 覆盖 grpcDialer：按地址返回对应 bufconn 连接
	origDialer := grpcDialer
	t.Cleanup(func() { grpcDialer = origDialer })
	grpcDialer = func(addr string) (*grpc.ClientConn, error) {
		var lis *bufconn.Listener
		switch addr {
		case "user-buf":
			lis = userLis
		case "chat-buf":
			lis = chatLis
		case "assessment-buf":
			lis = assessmentLis
		case "analytics-buf":
			lis = analyticsLis
		default:
			return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		return grpc.NewClient("bufnet",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(bufDialer(lis)),
		)
	}

	// 构造 config：4 个下游都配 GRPCAddr，并显式 Transport=grpc（默认 http 是 Stage 63 收口后的
	// 临时降级；测试此用例是为了验证"Transport=grpc 时接线正确"，与 Stage 63 终态对齐）
	cfg := config.Config{}
	config.SetDefaults(&cfg)
	cfg.Auth.JWTSecret = "test-secret"
	cfg.UserService.GRPCAddr = "user-buf"
	cfg.UserService.Transport = "grpc"
	cfg.ChatService.GRPCAddr = "chat-buf"
	cfg.ChatService.Transport = "grpc"
	cfg.AssessmentService.GRPCAddr = "assessment-buf"
	cfg.AssessmentService.Transport = "grpc"
	cfg.AnalyticsService.GRPCAddr = "analytics-buf"
	cfg.AnalyticsService.Transport = "grpc"

	svcCtx := buildServiceContext(&cfg, nil)

	// 断言 4 个 client 都是 gRPC 实现
	isGRPCClient(t, svcCtx.User)
	isGRPCClient(t, svcCtx.Chat)
	isGRPCClient(t, svcCtx.Assessment)
	isGRPCClient(t, svcCtx.Analytics)
}

// TestBuildServiceContext_TransportHTTP_ForcesHTTPFallback 回滚开关测试：
// 当 Transport="http" 时，即使 GRPCAddr 配置了也应走 HTTP client。
func TestBuildServiceContext_TransportHTTP_ForcesHTTPFallback(t *testing.T) {
	origDialer := grpcDialer
	t.Cleanup(func() { grpcDialer = origDialer })
	grpcDialer = func(addr string) (*grpc.ClientConn, error) {
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	cfg := config.Config{}
	config.SetDefaults(&cfg)
	cfg.Auth.JWTSecret = "test-secret"
	cfg.UserService.GRPCAddr = "localhost:8887"
	cfg.UserService.Transport = "http"
	cfg.ChatService.GRPCAddr = "localhost:8892"
	cfg.ChatService.Transport = "http"

	svcCtx := buildServiceContext(&cfg, nil)

	isHTTPClient(t, svcCtx.User)
	isHTTPClient(t, svcCtx.Chat)
}

// TestBuildServiceContext_GRPCAddrEmpty_HTTPFallback 兜底测试：
// GRPCAddr 为空时不 dial，返回 HTTP client。
func TestBuildServiceContext_GRPCAddrEmpty_HTTPFallback(t *testing.T) {
	cfg := config.Config{}
	config.SetDefaults(&cfg)
	cfg.Auth.JWTSecret = "test-secret"
	cfg.UserService.GRPCAddr = ""
	cfg.ChatService.GRPCAddr = ""
	cfg.AssessmentService.GRPCAddr = ""
	cfg.AnalyticsService.GRPCAddr = ""

	svcCtx := buildServiceContext(&cfg, nil)

	isHTTPClient(t, svcCtx.User)
	isHTTPClient(t, svcCtx.Chat)
	isHTTPClient(t, svcCtx.Assessment)
	isHTTPClient(t, svcCtx.Analytics)
}

// TestBuildServiceContext_DialFailure_HTTPFallback 容错测试：
// dial 失败时不 panic，返回 HTTP client（降级）。
func TestBuildServiceContext_DialFailure_HTTPFallback(t *testing.T) {
	origDialer := grpcDialer
	t.Cleanup(func() { grpcDialer = origDialer })
	grpcDialer = func(addr string) (*grpc.ClientConn, error) {
		return nil, assert.AnError
	}

	cfg := config.Config{}
	config.SetDefaults(&cfg)
	cfg.Auth.JWTSecret = "test-secret"
	cfg.UserService.GRPCAddr = "invalid:1234"

	svcCtx := buildServiceContext(&cfg, nil)

	isHTTPClient(t, svcCtx.User)
}

// TestDialGRPC_EmptyAddr_ReturnsNil 单元测试：GRPCAddr 为空时 dialGRPC 返 nil（不 dial）。
func TestDialGRPC_EmptyAddr_ReturnsNil(t *testing.T) {
	conn := dialGRPC("", "test-svc")
	assert.Nil(t, conn, "空地址应返 nil，不 dial")
}

// TestDialGRPC_InvalidAddr_ReturnsNil 单元测试：dial 失败时返 nil（不 panic）。
func TestDialGRPC_InvalidAddr_ReturnsNil(t *testing.T) {
	origDialer := grpcDialer
	t.Cleanup(func() { grpcDialer = origDialer })
	grpcDialer = func(addr string) (*grpc.ClientConn, error) {
		return nil, assert.AnError
	}

	conn := dialGRPC("invalid:1234", "test-svc")
	assert.Nil(t, conn, "dial 失败应返 nil（降级 HTTP）")
	require.NotPanics(t, func() {
		dialGRPC("invalid:1234", "test-svc")
	}, "dial 失败不应 panic")
}
