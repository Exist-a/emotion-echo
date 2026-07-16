// Package skywalking 提供 SkyWalking 链路追踪的初始化。
//
// 设计要点：
//   - 仅需要初始化一次（main.go 中调用 Init）
//   - 通过 Reporter.NewGRPCReporter 上报到 SkyWalking OAP
//   - 真正的 http 中间件由 plugins/http/server.go 提供
package skywalking

import (
	"log"
	"os"
	"sync"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
)

const (
	// 默认 OAP gRPC 端口（dev 环境为宿主机 localhost:11800）
	defaultOAPAddr = "localhost:11800"

	// 服务名（SkyWalking UI 上显示的 Name 列）
	defaultServiceName = "emotion-echo-gin"
)

var (
	once   sync.Once
	tracer *go2sky.Tracer
	rep    go2sky.Reporter
)

// Init 初始化全局 tracer。
// 调用完成后才能使用 Tracer()。
func Init() {
	once.Do(initTracer)
}

// Tracer 返回全局 tracer，未初始化时返回 nil。
func Tracer() *go2sky.Tracer {
	return tracer
}

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
	tracer = tr

	log.Printf("[skywalking] tracer initialized, oap=%s service=%s\n", oapAddr, serviceName)
}
