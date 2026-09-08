// Package bootstrap — SkyWalking tracer init helper（PR-OBS-2）
//
// 设计动机：
//   - 5 svc 之前 `tracer, _ = go2sky.NewTracer(...)` 静默吞错,
//     SkyWalking OAP 不可达时无人能感知。
//   - BootstrapSkyWalkingTracer 统一 7 svc 的 tracer 初始化行为：
//     1. ctx 必填（nil → fail-fast 返回 error，不 panic）
//     2. CheckTCP 验证 OAP gRPC port 11800 可达（不依赖真实 gRPC dial）
//     3. reporter.NewGRPCReporter + go2sky.NewTracer 任一步失败 → 返回 ErrDial/ErrReporter/ErrTracer 包装
//     4. 调用方按 IsRequired("skywalking") + ShouldFailFast() 决定 fail-fast vs warn
//     5. 失败时由调用方负责 IncSkyWalkingInitFailed(svc)（metrics 在 shared/pkg/metrics，
//        本包不引入 metrics 依赖，避免循环）
//
// 与 CheckTCP 的关系：CheckTCP 只 TCP 探活，不做 gRPC handshake。本 helper 在
// TCP 通的基础上再走 gRPC reporter dial — 两步都失败才返回 error。
package bootstrap

import (
	"context"
	"time"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
)

// BootstrapSkyWalkingTracer 初始化 SkyWalking tracer
//
// 参数：
//   - ctx: 必填，传 appCtx 或 background(WithTimeout)
//   - serviceName: 上报 OAP 的 service 名（如 "chat-svc"）
//   - oapAddr: OAP gRPC receiver 地址（如 "emotion-echo-sw-oap:11800"）
//   - dialTimeout: TCP 探活超时（建议 2-3s，不要太长）
//
// 返回：
//   - tracer: 成功时非 nil，调用方负责挂到 GinSkywalkingMiddleware / gRPC 拦截器
//   - err: 失败时非 nil，含详细原因（nil ctx / dial fail / reporter / tracer）
//     调用方按 IsRequired("skywalking") + ShouldFailFast() 决定退出 vs warn
func BootstrapSkyWalkingTracer(ctx context.Context, serviceName, oapAddr string, dialTimeout time.Duration) (*go2sky.Tracer, error) {
	if ctx == nil {
		return nil, &ErrNilContext{Service: serviceName}
	}

	if err := CheckTCP(ctx, oapAddr, dialTimeout); err != nil {
		return nil, err // CheckTCP 已返回 ErrDial
	}

	rep, err := reporter.NewGRPCReporter(oapAddr)
	if err != nil {
		return nil, &ErrReporterInit{Service: serviceName, Addr: oapAddr, Err: err}
	}

	tracer, err := go2sky.NewTracer(serviceName, go2sky.WithReporter(rep))
	if err != nil {
		return nil, &ErrTracerInit{Service: serviceName, Err: err}
	}
	return tracer, nil
}

// ErrNilContext ctx 为 nil 时返回（fail-fast 不 panic）
type ErrNilContext struct {
	Service string
}

func (e *ErrNilContext) Error() string {
	return "BootstrapSkyWalkingTracer: nil context for service " + e.Service
}

// ErrReporterInit reporter.NewGRPCReporter 失败
type ErrReporterInit struct {
	Service string
	Addr    string
	Err     error
}

func (e *ErrReporterInit) Error() string {
	return "skywalking reporter init failed for " + e.Service + " (" + e.Addr + "): " + e.Err.Error()
}

func (e *ErrReporterInit) Unwrap() error { return e.Err }

// ErrTracerInit go2sky.NewTracer 失败
type ErrTracerInit struct {
	Service string
	Err     error
}

func (e *ErrTracerInit) Error() string {
	return "skywalking tracer init failed for " + e.Service + ": " + e.Err.Error()
}

func (e *ErrTracerInit) Unwrap() error { return e.Err }
