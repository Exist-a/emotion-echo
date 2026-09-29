// Package grpcserver 提供 user-svc 的 gRPC 服务（Stage 62 PR-3.2）
//
// 与 HTTP server（Gin :8888）共存：
//   - HTTP 给前端（Nuxt）/ APISIX 网关
//   - gRPC 给内部 BFF（PR-3.3 阶段落地）
//
// 实现 UserService（proto/user.proto）：
//   - GetMe / UpdateProfile / GetUserById
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

	"emotion-echo-user-svc/internal/svc"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const healthServiceFullName = "/grpc.health.v1.Health"

// emotionQueryServiceName 是本服务在 gRPC health 中注册的业务 service 名。
// 抽成常量：注册与停机翻转两处必须用同一个值，否则翻转漏掉业务 service。
const emotionQueryServiceName = "emotion.User"

// newServiceAwareUserIDInterceptor 跳过 health probe + Login/Register 的 user id 检查
//
// 跳过清单：
//   - health check（k8s probe 不带 x-user-id metadata）
//   - Login / Register（匿名调用，调用方无身份；user-svc 自己用 username/password 鉴权）
//
// Sprint E（2026-09-11）：补 Login/Register 跳过。实现用 method name 白名单（精确匹配），
// 避免 prefix 误伤未来新增的 RPC。
func newServiceAwareUserIDInterceptor(skipServiceFullName string, anonMethods ...string) grpc.UnaryServerInterceptor {
	skipSet := make(map[string]bool, len(anonMethods)+1)
	for _, m := range anonMethods {
		skipSet[m] = true
	}
	inner := grpcinterceptor.NewServerUserIDInterceptor()
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info != nil && info.FullMethod != "" {
			if strings.HasPrefix(info.FullMethod, skipServiceFullName) {
				return handler(ctx, req)
			}
			if skipSet[info.FullMethod] {
				return handler(ctx, req)
			}
		}
		return inner(ctx, req, info, handler)
	}
}

// Server user-svc 的 gRPC server
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
//
// svcCtx 传 nil 时 logic 层会触发 nil 指针 — PR-3.2 阶段允许"裸启动"
// （用于验证 gRPC 链路通；PR-3.3 阶段 BFF 真实接入后再考虑注入 svcCtx）。
func New(svcCtx *svc.ServiceContext, port int) *Server {
	tracer := skywalking.Tracer()
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			newServiceAwareUserIDInterceptor(
				healthServiceFullName,
				emotionuser.UserService_Login_FullMethodName,
				emotionuser.UserService_Register_FullMethodName,
				emotionuser.UserService_ResetPassword_FullMethodName,
				// R-01 #2：密保校验是找回密码的入口，调用者尚未登录、只有用户名，
				// 故 proto（user.proto:「鉴权：匿名调用（同 Login/Register）」）与
				// HTTP 端（user-svc/main.go 的 noAuth.POST）都把它定义为匿名。
				// 此前这里漏配 ⇒ 不带 x-user-id 调用被拦截器以 Unauthenticated 拒掉，
				// 即"契约说匿名、实现要求带身份"，密保校验在 gRPC 路径上实际不可用。
				emotionuser.UserService_VerifySecurityAnswer_FullMethodName,
				emotionuser.UserService_VerifySecurityAnswerByUsername_FullMethodName,
				// E2E-07：获取密保问题（找回密码流程，调用者未登录）
				emotionuser.UserService_GetSecurityQuestionsByUsername_FullMethodName,
			),
			grpcinterceptor.NewServerTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(tracer)),
			grpcinterceptor.ServerLoggingInterceptor(),
			grpcinterceptor.ServerRecoveryInterceptor(),
		),
	}
	gs := grpc.NewServer(opts...)

	// 注册 UserService
	emotionuser.RegisterUserServiceServer(gs, &userServer{svcCtx: svcCtx})

	// 注册 health check（跳过 user id 校验）
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
	logging.PrintfContext(ctx, "[grpc] user-svc gRPC server listening on :%d", s.port)
	logging.PrintfContext(ctx, "[grpc] services: UserService (user id required)")

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
