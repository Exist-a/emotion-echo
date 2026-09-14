// Package grpcclient — ai_client_grpc_test.go
//
// Sibling test for ai_client_grpc.go (per AGENTS.md §1.1).
//
// Stage 94 PR-2 §P0-2 钉死契约：chat-svc → ai-svc gRPC dial 必须经
// sharedgrpc.ClientDialOptions helper 装配 tracing/timeout/logging 链,
// 而不是裸 grpc.DialContext（§P0-2 原文: "chat-svc → ai-svc gRPC client
// 完全无拦截器（无 trace / 无 retry / 无 timeout）"）。
//
// 测试方式（与 ai-svc/internal/analyzer/grpc_analyzer_test.go
// TestGRPCAnalyzer_UsesClientDialOptionsHelper 同模式）：源码字面量断言。
// 理由:
//   - e2e bufconn 验证 sw8 metadata 端到端已在 ai-svc 测试覆盖
//   - 本测试钉死"必须用 helper 而非 inline 4 个 interceptor",防漂移
//   - 与 Stage 92 §"tag 字面量未漂移"风格一致
package grpcclient

import (
	"os"
	"strings"
	"testing"
)

// TestAIgRPCClient_UsesClientDialOptionsHelper §P0-2 钉死契约:
//
// ai_client_grpc.go 的 dial options 必须经 sharedgrpc.ClientDialOptions helper,
// 而不是裸 grpc.DialContext (无 interceptor)。
func TestAIgRPCClient_UsesClientDialOptionsHelper(t *testing.T) {
	srcBytes, err := os.ReadFile("ai_client_grpc.go")
	if err != nil {
		t.Skipf("cannot read ai_client_grpc.go: %v", err)
	}
	src := string(srcBytes)

	wantContain := []string{
		// 必须用 helper（与 web-bff PR-4 / ai-svc→llm-service 同模式）
		`grpcinterceptor.ClientDialOptions(`,
		// 必须给 helper 传 tracer —— 用 go2sky adapter 包装（生产模式）
		`grpcinterceptor.NewGo2SkyTracer(`,
		// 必须传 timeout（防 handler 忘了设 deadline 时永久阻塞）
		`5*time.Second`,
	}
	for _, w := range wantContain {
		if !strings.Contains(src, w) {
			t.Errorf("ai_client_grpc.go 缺 %q — §P0-2 修复要求用 shared ClientDialOptions helper 而非裸 grpc.DialContext",
				w)
		}
	}
}