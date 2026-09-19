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
	"log"
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
	healthSrv.SetServingStatus("emotion.User", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, healthSrv)

	return &Server{
		grpcServer: gs,
		port:       port,
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
	log.Printf("[grpc] user-svc gRPC server listening on :%d", s.port)
	log.Printf("[grpc] services: UserService (user id required)")

	go func() {
		<-ctx.Done()
		log.Printf("[grpc] shutting down...")
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
