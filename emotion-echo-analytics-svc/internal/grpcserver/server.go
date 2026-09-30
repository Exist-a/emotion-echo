// Package grpcserver 提供 analytics-svc 的 gRPC 服务（Stage 62 PR-3.2）
//
// 与 HTTP server（Gin :8893）共存：
//   - HTTP 给前端（Nuxt）/ APISIX 网关
//   - gRPC 给内部 BFF（PR-3.3 阶段落地）
//
// 实现 AnalyticsService（proto/metric.proto）：
//   - ReportsDaily / ReportsTrend
//   - UserBehaviorDayNight / Depth / Frequency
//   - MentalHealthAssessment / History / Trigger / Trend
//
// 拦截器链（复用 shared/pkg/grpcinterceptor/）：
//   - user id metadata 拦截器（APISIX 注入 x-user-id）
//   - tracing 拦截器（SkyWalking OAP）
//   - logging + recovery
//
// 健康检查走 grpc.health.v1（gRPC 标准；k8s probe 用）。
package grpcserver

import (
	"context"
	"fmt"

	"github.com/emotion-echo/shared/pkg/logging"
	"net"
	"strings"
	"sync"

	"emotion-echo-analytics-svc/internal/svc"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const healthServiceFullName = "/grpc.health.v1.Health"

// emotionQueryServiceName 是本服务在 gRPC health 中注册的业务 service 名。
// 抽成常量：注册与停机翻转两处必须用同一个值，否则翻转漏掉业务 service。
const emotionQueryServiceName = "emotion_analytics.v1.AnalyticsService"

// ⚠️ 这里的值必须是 **proto 生成的 ServiceDesc.ServiceName 原样**
//    （可在 emotion-echo-shared 的 *_grpc.pb.go 里 grep 'ServiceName: "emotion_analytics.v1.AnalyticsService"' 复核）。
// E2E-23 复核轮更正：本常量此前写的是 "emotion.Analytics" —— **proto 里根本不存在这个名字**，
//    导致 per-service 健康只有测试自己查得到，真实 gRPC 客户端一律 NOT_FOUND，
//    而测试用同一字面量断言形成自证循环，永远绿。
//    回归钉：scripts/test_grpc_health_shutdown.sh 第 6 条。

func newServiceAwareUserIDInterceptor(skipServiceFullName string) grpc.UnaryServerInterceptor {
	inner := grpcinterceptor.NewServerUserIDInterceptor()
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info != nil && info.FullMethod != "" {
			if strings.HasPrefix(info.FullMethod, skipServiceFullName) {
				return handler(ctx, req)
			}
		}
		return inner(ctx, req, info, handler)
	}
}

// Server analytics-svc 的 gRPC server
type Server struct {
	// mu 保护 listener：Start() 在后台 goroutine 里写入，Addr() 会被
	// 调用方（含测试的轮询）并发读取 —— 无同步即为真实数据竞争
	// （CI/dev 用 -race 实测到 WARNING: DATA RACE）。
	mu         sync.RWMutex
	grpcServer *grpc.Server
	listener   net.Listener
	port       int
	// healthSrv 必须存下来：停机时要把所有 service 翻成 NOT_SERVING，
	// 让上游在连接真正关闭前先摘掉流量。此前它是 New() 里的局部变量，
	// 停机分支拿不到 —— 于是 health 永远停在 SERVING（E2E-23 测试点 #13）。
	healthSrv *health.Server
}

// MarkShuttingDown 把所有已注册 service 的健康状态翻为 NOT_SERVING。
//
// 生产路径：main 收到 SIGTERM/SIGINT 后、GracefulStop 之前调用。
// 顺序很关键 —— 先翻状态（上游摘流量），再 GracefulStop（等在途 RPC 结束），
// 反过来会让停机窗口内的请求被丢在正在关闭的实例上。
//
// 幂等：停机信号可能重复送达（SIGTERM 后 SIGKILL、重启重试等），
// 重复调用只是重复写入同一状态，grpc 的 health.Server 内部有锁。
func (s *Server) MarkShuttingDown() {
	s.mu.RLock()
	hs := s.healthSrv
	s.mu.RUnlock()
	if hs == nil {
		return
	}
	for _, svcName := range []string{"", emotionQueryServiceName} {
		hs.SetServingStatus(svcName, healthpb.HealthCheckResponse_NOT_SERVING)
	}
}

// New 创建并配置 gRPC server（未启动）
func New(svcCtx *svc.ServiceContext, port int) *Server {
	tracer := skywalking.Tracer()
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			newServiceAwareUserIDInterceptor(healthServiceFullName),
			grpcinterceptor.NewServerTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(tracer)),
			grpcinterceptor.ServerLoggingInterceptor(),
			grpcinterceptor.ServerRecoveryInterceptor(),
		),
	}
	gs := grpc.NewServer(opts...)

	// 注册 AnalyticsService
	emotionanalytics.RegisterAnalyticsServiceServer(gs, &analyticsServer{svcCtx: svcCtx})

	// 注册 health check
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus(emotionQueryServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, healthSrv)

	return &Server{
		grpcServer: gs,
		port:       port,
		healthSrv:  healthSrv,
	}
}

// Start 监听并启动 server（阻塞直到 ctx 取消）
func (s *Server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("listen :%d: %w", s.port, err)
	}
	s.mu.Lock()
	s.listener = lis
	s.mu.Unlock()
	logging.PrintfContext(ctx, "[grpc] analytics-svc gRPC server listening on :%d", s.port)
	logging.PrintfContext(ctx, "[grpc] services: AnalyticsService (user id required)")

	go func() {
		<-ctx.Done()
		logging.PrintfContext(ctx, "[grpc] shutting down...")
		// 先翻 health 状态再关连接：让上游能在连接真正断开前摘掉流量。
		// 顺序反了会让停机窗口内的请求打进正在关闭的实例。
		s.MarkShuttingDown()
		s.grpcServer.GracefulStop()
	}()

	return s.grpcServer.Serve(lis)
}

// Addr 返回监听地址
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return fmt.Sprintf(":%d", s.port)
	}
	return s.listener.Addr().String()
}
