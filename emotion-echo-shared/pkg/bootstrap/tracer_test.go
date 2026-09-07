package bootstrap

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/SkyAPM/go2sky"
)

// PR-OBS-2 RED: BootstrapSkyWalkingTracer 应统一 7 svc 的 tracer 初始化。
//
// 设计动机：
//   - 现状：5 svc (chat/assessment/analytics/web-bff/ai-svc) 全是 `tracer, _ = go2sky.NewTracer(...)`
//     (静默吞错,skywalking dial fail 看不见)
//   - user-svc 是唯一记 err 但只 log 不退出 — 同样看不见
//   - PR-OBS-2 目标：抽 BootstrapSkyWalkingTracer,统一行为:
//     * 启动前 CheckTCP 验证 OAP 可达(沿用 deps.go CheckTCP)
//     * reporter.NewGRPCReporter + go2sky.NewTracer 失败返回 error
//     * 调用方按 IsRequired("skywalking") + ShouldFailFast() 决定 fail-fast / warn
//     * metrics counter emotion_echo_skywalking_init_failed_total{service} 递增

func TestBootstrapSkyWalkingTracer_LiveAddr_ReturnsTracer(t *testing.T) {
	// 起一个临时 listener 模拟 OAP gRPC port 11800
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	// BootstrapSkyWalkingTracer 现在不存在 → 编译失败 (RED 状态)
	tracer, err := BootstrapSkyWalkingTracer(context.Background(), "test-svc", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("BootstrapSkyWalkingTracer(%s): unexpected err: %v", addr, err)
	}
	if tracer == nil {
		t.Fatal("BootstrapSkyWalkingTracer returned nil tracer on live addr")
	}
	// go2sky.NewTracer 返回的实例需能 Close（不强制验证,但断言非零指针即可）
	_ = go2sky.Tracer(*tracer)
}

func TestBootstrapSkyWalkingTracer_DeadAddr_ReturnsDialErr(t *testing.T) {
	// 找一个肯定关闭的端口（端口 1 通常未绑定）
	const deadAddr = "127.0.0.1:1"
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	tracer, err := BootstrapSkyWalkingTracer(ctx, "test-svc", deadAddr, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected dial error on dead addr, got nil")
	}
	if tracer != nil {
		t.Errorf("expected nil tracer on dial fail, got %v", tracer)
	}
	// 错误应包含 addr 信息,便于排查
	if !strings.Contains(err.Error(), deadAddr) {
		t.Errorf("error message should contain addr %q, got: %v", deadAddr, err)
	}
}

func TestBootstrapSkyWalkingTracer_NilContext_ReturnsErr(t *testing.T) {
	// nil ctx 应 fail-fast 返回 error,不 panic
	tracer, err := BootstrapSkyWalkingTracer(nil, "test-svc", "127.0.0.1:1", 100*time.Millisecond)
	if err == nil {
		t.Error("expected error on nil ctx, got nil")
	}
	if tracer != nil {
		t.Errorf("expected nil tracer on nil ctx, got %v", tracer)
	}
}
