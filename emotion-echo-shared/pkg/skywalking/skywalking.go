// Package skywalking 提供 SkyWalking 链路追踪的全局 tracer 注册。
//
// 设计要点：
//   - 各 svc main.go 用 shared/pkg/bootstrap.BootstrapSkyWalkingTracer 初始化，
//     成功后调 SetTracer(t) 注册到本包，供 skywalking.Tracer() 消费方
//     （5 个 gRPC server 拦截器、chat/ai gRPC client 的 traceTracer()）
//   - 历史 Init()/initTracer() 读废弃 env（SKY_SW_OAP_ADDR）且全仓零调用，
//     属死路径（E2E-26 计划期核实），保留仅为兼容，新代码禁止调用
//   - 通过 Reporter.NewGRPCReporter 上报到 SkyWalking OAP 的职责在 bootstrap 包
package skywalking

import (
	"log"
	"os"
	"sync"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"

	"github.com/emotion-echo/shared/pkg/skywalking/trace"
)

const (
	// 默认 OAP gRPC 端口（dev 环境为宿主机 localhost:11800）
	defaultOAPAddr = "localhost:11800"

	// 服务名（SkyWalking UI 上显示的 Name 列）
	defaultServiceName = "emotion-echo-gin"
)

// 注：全局 tracer 注册表已拆到 pkg/skywalking/trace（**无 gorm/redis 依赖**，
// 供 web-bff 等轻量调用方使用）。本包保留 SetTracer/Tracer forwarder，既有 svc 零改动。
var (
	once sync.Once
	rep  go2sky.Reporter
)

// SetTracer 注册（或清空）全局 tracer，供 Tracer() 读取 —— forwarder 到
// pkg/skywalking/trace（注册表已拆到该无 gorm 依赖的包）。
//
// E2E-26 #3：BootstrapSkyWalkingTracer 成功后必须调用本函数，否则取全局
// tracer 的 gRPC server/client 全部拿到 nil → noop → 服务端 span 与 sw8
// 提取静默失效（单测 mock 注入掩盖，运行时全灭）。
func SetTracer(t *go2sky.Tracer) { trace.SetTracer(t) }

// Init 初始化全局 tracer。
// 调用完成后才能使用 Tracer()。
//
// 已废弃（E2E-26）：全仓无调用方且读废弃 env；新代码用
// bootstrap.BootstrapSkyWalkingTracer + SetTracer。
func Init() {
	once.Do(initTracer)
}

// Tracer 返回全局 tracer，未初始化时返回 nil。
func Tracer() *go2sky.Tracer { return trace.Tracer() }

// Shutdown 释放 reporter 连接。
func Shutdown() {
	if rep != nil {
		rep.Close()
	}
}

func initTracer() {
	oapAddr := os.Getenv("SKY_SW_OAP_ADDR")
	if oapAddr == "" {
		oapAddr = defaultOAPAddr
	}

	serviceName := os.Getenv("SKY_SERVICE_NAME")
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	// gRPC 上报到 OAP
	r, err := reporter.NewGRPCReporter(oapAddr)
	if err != nil {
		log.Printf("[skywalking] failed to create grpc reporter (oap=%s): %v\n", oapAddr, err)
		return
	}
	rep = r

	tr, err := go2sky.NewTracer(
		serviceName,
		go2sky.WithReporter(r),
	)
	if err != nil {
		log.Printf("[skywalking] failed to create tracer: %v\n", err)
		r.Close()
		rep = nil
		return
	}
	SetTracer(tr)

	log.Printf("[skywalking] tracer initialized, oap=%s service=%s\n", oapAddr, serviceName)
}
