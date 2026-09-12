// Package main — main_grpc_discovery_test.go
//
// Stage 75 RED: BFF gRPC 拨号地址走 Nacos Discover。
//
// 背景（Stage 63 后遗留）：dev 默认 Transport=grpc（决策 4），但 buildServiceContext 的
// 5 处 dialGRPC 全部用 env 注入的容器 DNS（*_SVC_GRPC_ADDR）——PR-2 已有的 Resolver 只
// 影响 HTTP 路径（BaseURL 为空时才生效，而 compose 总是注入 URL），gRPC 流量从未经过
// Nacos。根因之一是 user/chat/assessment/analytics 4 个 svc 注册时缺 metadata.grpc_port
// （仅 ai-svc 有），BFF 即使想解析也无端口可查。
//
// 本文件断言 web-bff 侧契约：
//  1. resolveGRPCAddr：Nacos 有实例 → Nacos 地址优先；Discover 空/失败/返回空 host →
//     env GRPCAddr 兜底（Nacos 抖动不阻断启动）；resolver 为 nil（NACOS_ENABLED=false）→
//     行为与 Stage 63 完全一致。
//  2. buildServiceContext：grpcResolver 非 nil 时 5 处 gRPC 拨号地址全部来自 Resolver，
//     且 EmotionQuery 连接不再被"无 portHint 的 HTTP resolver"覆盖成 8891（Stage 75
//     顺手修复的潜伏 bug：原代码用无 portHint resolver 解析 ai-svc，拿到 HTTP 端口
//     覆盖正确的 env AI_SVC_GRPC_ADDR）。
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/svc"
)

// stubNacosResolve 固定返回 host/port/err 的 Resolver 替身（Resolve 调用记录服务名）。
type stubNacosResolve struct {
	host  string
	port  int
	err   error
	calls []string
}

func (s *stubNacosResolve) Resolve(_ context.Context, svcName string) (string, int, error) {
	s.calls = append(s.calls, svcName)
	return s.host, s.port, s.err
}

func TestResolveGRPCAddr_NacosInstanceWins(t *testing.T) {
	r := &stubNacosResolve{host: "from-nacos", port: 8892}
	got := resolveGRPCAddr(r, "env-fallback:8892", shareddiscovery.ServiceChat)
	assert.Equal(t, "from-nacos:8892", got)
	assert.Equal(t, []string{shareddiscovery.ServiceChat}, r.calls)
}

func TestResolveGRPCAddr_DiscoverErrorFallsBackToEnv(t *testing.T) {
	r := &stubNacosResolve{err: assert.AnError}
	got := resolveGRPCAddr(r, "env-fallback:8892", shareddiscovery.ServiceChat)
	assert.Equal(t, "env-fallback:8892", got)
}

func TestResolveGRPCAddr_EmptyHostFallsBackToEnv(t *testing.T) {
	r := &stubNacosResolve{host: "", port: 0}
	got := resolveGRPCAddr(r, "env-fallback:8892", shareddiscovery.ServiceChat)
	assert.Equal(t, "env-fallback:8892", got)
}

func TestResolveGRPCAddr_NilResolverKeepsStage63Behavior(t *testing.T) {
	got := resolveGRPCAddr(nil, "env-fallback:8892", shareddiscovery.ServiceChat)
	assert.Equal(t, "env-fallback:8892", got)
}

// TestBuildServiceContext_GRPCDial_AddrsResolvedViaNacos：grpcResolver 非 nil 时
// 5 处 gRPC 拨号地址（user/chat/assessment/analytics/ai EmotionQuery）全部来自
// Resolver，env GRPCAddr 退为兜底。
func TestBuildServiceContext_GRPCDial_AddrsResolvedViaNacos(t *testing.T) {
	origDialer := grpcDialer
	t.Cleanup(func() { grpcDialer = origDialer })
	var dialed []string
	grpcDialer = func(addr string) (*grpc.ClientConn, error) {
		dialed = append(dialed, addr)
		// 连接是惰性的，真实地址拨不上不影响断言
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	cfg := config.Config{}
	config.SetDefaults(&cfg)
	cfg.Auth.JWTSecret = "test-secret"
	cfg.UserService.GRPCAddr = "env-user:8887"
	cfg.ChatService.GRPCAddr = "env-chat:8892"
	cfg.AssessmentService.GRPCAddr = "env-assessment:8886"
	cfg.AnalyticsService.GRPCAddr = "env-analytics:8885"
	cfg.AIService.GRPCAddr = "env-ai:8892"

	grpcRes := &stubNacosResolve{host: "nacos-host", port: 18888}
	var svcCtx *svc.ServiceContext
	require.NotPanics(t, func() {
		svcCtx = buildServiceContext(&cfg, nil, grpcRes)
	})
	require.NotNil(t, svcCtx)

	// 5 处 gRPC 拨号全部命中 Nacos 地址，env 值一个都没被用
	require.Len(t, dialed, 5, "user/chat/assessment/analytics + ai EmotionQuery")
	for _, addr := range dialed {
		assert.Equal(t, "nacos-host:18888", addr)
	}
}

// TestBuildServiceContext_GRPCDial_NilGrpcResolverKeepsEnv：nil grpcResolver
// （NACOS_ENABLED=false）时拨号地址与 Stage 63 行为逐字节一致。
func TestBuildServiceContext_GRPCDial_NilGrpcResolverKeepsEnv(t *testing.T) {
	origDialer := grpcDialer
	t.Cleanup(func() { grpcDialer = origDialer })
	var dialed []string
	grpcDialer = func(addr string) (*grpc.ClientConn, error) {
		dialed = append(dialed, addr)
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	cfg := config.Config{}
	config.SetDefaults(&cfg)
	cfg.Auth.JWTSecret = "test-secret"
	cfg.UserService.GRPCAddr = "env-user:8887"
	cfg.ChatService.GRPCAddr = "env-chat:8892"
	cfg.AssessmentService.GRPCAddr = "env-assessment:8886"
	cfg.AnalyticsService.GRPCAddr = "env-analytics:8885"
	cfg.AIService.GRPCAddr = "env-ai:8892"

	buildServiceContext(&cfg, nil, nil)

	require.Len(t, dialed, 5)
	assert.Equal(t, []string{
		"env-user:8887", "env-chat:8892", "env-assessment:8886",
		"env-analytics:8885", "env-ai:8892",
	}, dialed)
}
