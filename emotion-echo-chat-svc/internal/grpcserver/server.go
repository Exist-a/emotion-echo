// Package grpcserver 提供 chat-svc 的 gRPC 服务（Stage 58 PR-GRPC-2）
//
// 与 HTTP server（Gin :8890）共存：
//   - HTTP 给前端（Nuxt）/ APISIX 网关
//   - gRPC 给内部 BFF（Phase 1 迁移目标，plan §二决策 A）
//
// 实现 ChatService（proto/chat.proto）：
//   - CreateConversation / SendMessage / ListMessages / ListConversations
//   - DeleteConversation / PinConversation
//   - StreamMessages (server stream) — 替代 SSE 聊天流
//
// 拦截器链（复用 shared/pkg/grpcinterceptor/）：
//   - user id metadata 拦截器（APISIX 注入 x-user-id）
//   - tracing 拦截器（SkyWalking OAP）
//   - logging + recovery
//
// 健康检查走 grpc.health.v1（gRPC 标准）。
package grpcserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"

	"emotion-echo-chat-svc/internal/svc"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const healthServiceFullName = "/grpc.health.v1.Health"

// newServiceAwareUserIDInterceptor 跳过 health probe 的 user id 检查
//（k8s probe 不带 x-user-id metadata）
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

// Server chat-svc 的 gRPC server
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	port       int
}

// New 创建并配置 gRPC server（未启动）
//
// svcCtx 传 nil 时 logic 层会触发 nil 指针 — 这是 PR-GRPC-2 阶段允许的"裸启动"
// （用于验证 gRPC 链路通；PR-GRPC-3 阶段 main.go 会注入真实 svcCtx）。
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

	// 注册 ChatService
	emotionchat.RegisterChatServiceServer(gs, &chatServer{svcCtx: svcCtx})

	// 注册 health check（跳过 user id 校验）
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("emotion.Chat", healthpb.HealthCheckResponse_SERVING)
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
	s.listener = lis
	log.Printf("[grpc] chat-svc gRPC server listening on :%d", s.port)
	log.Printf("[grpc] services: ChatService (user id required)")

	go func() {
		<-ctx.Done()
		log.Printf("[grpc] shutting down...")
		s.grpcServer.GracefulStop()
	}()

	return s.grpcServer.Serve(lis)
}

// Addr 返回监听地址
func (s *Server) Addr() string {
	if s.listener == nil {
		return fmt.Sprintf(":%d", s.port)
	}
	return s.listener.Addr().String()
}