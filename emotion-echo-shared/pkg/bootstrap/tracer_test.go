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

// ── E2E-F-204 回归钉：tracer 初始化必须有**有限重试/退避** ──────────────────
//
// 现象（账本 E2E-F-204）：svc 启动早于 OAP 时，tracer init 一次失败即永久放弃
// （`warn mode, continue` 只告警不重试）⇒ OAP 之后起来，该 svc 仍永不上报，
// OAP listServices 长期缺服务。修法 = 有限重试 + 退避，封顶不无限阻塞启动。

func TestBootstrapSkyWalkingTracerRetry_LiveAddr_ReturnsTracer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()

	tracer, err := BootstrapSkyWalkingTracerRetry(
		context.Background(), "test-svc", ln.Addr().String(), 2*time.Second, 3, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("live addr should succeed, got err: %v", err)
	}
	if tracer == nil {
		t.Fatal("live addr should return non-nil tracer")
	}
}

func TestBootstrapSkyWalkingTracerRetry_DeadAddr_RetriesThenFails(t *testing.T) {
	const deadAddr = "127.0.0.1:1"
	const attempts = 3
	const backoff = 150 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	tracer, err := BootstrapSkyWalkingTracerRetry(ctx, "test-svc", deadAddr, 200*time.Millisecond, attempts, backoff)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("dead addr must ultimately return error（但必须重试后才放弃）")
	}
	if tracer != nil {
		t.Errorf("expected nil tracer, got %v", tracer)
	}
	// 单次尝试几乎是瞬时的；>= (attempts-1)×backoff 证明发生了退避重试。
	if min := time.Duration(attempts-1) * backoff; elapsed < min {
		t.Errorf("expected ≥ %v (retries+backoff), got %v ⇒ 未重试", min, elapsed)
	}
}

func TestBootstrapSkyWalkingTracerRetry_ContextCanceled_BailsOutEarly(t *testing.T) {
	const deadAddr = "127.0.0.1:1"
	const attempts = 50
	const backoff = 1 * time.Second // 若 ctx 不生效会阻塞 ~50s

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立刻取消

	start := time.Now()
	_, err := BootstrapSkyWalkingTracerRetry(ctx, "test-svc", deadAddr, 100*time.Millisecond, attempts, backoff)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("canceled ctx must return error")
	}
	if elapsed > 5*time.Second {
		t.Errorf("canceled ctx 应立即退出（负向：不得无限阻塞启动），实际 %v", elapsed)
	}
}

