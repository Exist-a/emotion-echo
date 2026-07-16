// Package grpcserver 提供 ai-svc 的 gRPC 服务（Stage 19）
//
// 与 HTTP server（Gin :8891）共存：
//   - HTTP 给前端（Nuxt）
//   - gRPC 给内部 svc-to-svc
//
// 实现 EmotionQueryService：
//   - GetEmotionByMessage(message_id) → Emotion
//   - GetEmotionByConversation(conversation_id) → EmotionList
package grpcserver

import (
	"context"
	"fmt"
	"net"

	"emotion-echo-ai-svc/internal/logging"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Server ai-svc 的 gRPC server
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	port       int
}

// New 创建并配置 gRPC server（未启动）
func New(repo repository.EmotionRepo, port int) *Server {
	// Health check（标准 grpc.health.v1，用 grpc-go 自带 health.Server）
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("emotion.AI", healthpb.HealthCheckResponse_SERVING)

	// Interceptor 链
	tracer := skywalking.Tracer()
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			grpcinterceptor.NewServerTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(tracer)),
			grpcinterceptor.ServerLoggingInterceptor(),
			grpcinterceptor.ServerRecoveryInterceptor(),
		),
	}

	gs := grpc.NewServer(opts...)

	// 注册 service
	emotionquery.RegisterEmotionQueryServiceServer(gs, &emotionQueryServer{repo: repo})
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
	logging.Printf("[grpc] ai-svc gRPC server listening on :%d", s.port)
	logging.Printf("[grpc] services: EmotionQueryService + grpc.health.v1")

	go func() {
		<-ctx.Done()
		logging.Printf("[grpc] shutting down...")
		s.grpcServer.GracefulStop()
	}()

	return s.grpcServer.Serve(lis)
}

// Addr 返回监听地址（用于 e2e 测试）
func (s *Server) Addr() string {
	if s.listener == nil {
		return fmt.Sprintf(":%d", s.port)
	}
	return s.listener.Addr().String()
}

// emotionQueryServer 实现 EmotionQueryService
type emotionQueryServer struct {
	emotionquery.UnimplementedEmotionQueryServiceServer
	repo repository.EmotionRepo
}

func (s *emotionQueryServer) GetEmotionByMessage(ctx context.Context, req *emotionquery.GetEmotionByMessageRequest) (*emotionquery.Emotion, error) {
	if req.MessageId == 0 {
		return nil, status.Error(codes.InvalidArgument, "message_id is required")
	}
	e, err := s.repo.GetByMessageID(ctx, req.MessageId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	if e == nil {
		return nil, status.Error(codes.NotFound, "emotion not found for this message")
	}
	return toProtoEmotion(e), nil
}

func (s *emotionQueryServer) GetEmotionByConversation(ctx context.Context, req *emotionquery.GetEmotionByConversationRequest) (*emotionquery.EmotionList, error) {
	if req.ConversationId == 0 {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required")
	}
	limit := int(req.Limit)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.repo.ListByConversationID(ctx, req.ConversationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	items := make([]*emotionquery.Emotion, 0, len(rows))
	for i := range rows {
		items = append(items, toProtoEmotion(&rows[i]))
	}
	return &emotionquery.EmotionList{Items: items, Total: int32(len(items))}, nil
}

func toProtoEmotion(e *model.EmotionAnalysis) *emotionquery.Emotion {
	return &emotionquery.Emotion{
		Id:             e.ID,
		MessageId:      e.MessageID,
		ConversationId: e.ConversationID,
		PrimaryEmotion: e.PrimaryEmotion,
		SentimentScore: e.SentimentScore,
		Confidence:     e.Confidence,
		Model:          e.Model,
		CreatedAtMs:    e.CreatedAt.UnixMilli(),
	}
}
