// Package grpcserver 提供 ai-svc 的 gRPC 服务（Stage 19）
//
// 与 HTTP server（Gin :8891）共存：
//   - HTTP 给前端（Nuxt）/ 健康检查
//   - gRPC 给内部 svc-to-svc（EmotionQueryService）
//
// 实现 EmotionQueryService：
//   - GetEmotionByMessage(message_id) → Emotion
//   - GetEmotionByConversation(conversation_id) → EmotionList
//   - GetFusedEmotion(message_id) → FusedEmotion  (Stage 34)
//
// Stage 32 PR-16: 加 user ID metadata 拦截器（读 x-user-id，APISIX 注入）。
// 健康检查走 HTTP /api/v1/health（不在 gRPC 层做）。
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"emotion-echo-ai-svc/internal/aiclient"
	"emotion-echo-ai-svc/internal/logging"
	"emotion-echo-ai-svc/internal/logic"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"

	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// healthServiceFullName 是 grpc.health.v1.Health 服务的 FullMethod 前缀
//（gRPC FullMethod 形如 "/grpc.health.v1.Health/Check"，因此前缀带前导 /）
const healthServiceFullName = "/grpc.health.v1.Health"

// newServiceAwareUserIDInterceptor 包一层：根据 FullMethod service name 决定是否走 user id 校验
func newServiceAwareUserIDInterceptor(skipServiceFullName string) grpc.UnaryServerInterceptor {
	inner := grpcinterceptor.NewServerUserIDInterceptor()
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info != nil && info.FullMethod != "" {
			// FullMethod 形如 "/grpc.health.v1.Health/Check"
			// 跳过白名单 service（health probe 不带 x-user-id）
			if strings.HasPrefix(info.FullMethod, skipServiceFullName) {
				return handler(ctx, req)
			}
		}
		return inner(ctx, req, info, handler)
	}
}

// Server ai-svc 的 gRPC server
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	port       int
}

// New 创建并配置 gRPC server（未启动）
//
// Stage 34: 增加 fusedEmotionRepo 参数（查询 fused_emotions 表）。
// 现有调用方需要传 nil（向后兼容，单测已用 fake repo）。
//
// Sprint F2（2026-09-11）：加 svcCtx 参数，MultiModalAnalyze/SynthesizeSpeech/AIHealth
// 3 RPC 需要 svcCtx（构造 logic + aiclient）。现有 4 RPC（Get*/Upsert*）不依赖 svcCtx。
// 单测仍可传 nil（向前兼容）。
func New(repo repository.EmotionRepo, fusedEmotionRepo repository.FusedEmotionRepo, svcCtx *svc.ServiceContext, port int) *Server {
	// Interceptor 链
	tracer := skywalking.Tracer()
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			// Stage 32 PR-16: user ID metadata 拦截器（APISIX 注入 x-user-id）。
			// 业务 handler 用 grpcinterceptor.UserIDFromGRPCContext(ctx) 取 end user id。
			// 跳过 health check 服务（k8s probe 不带 x-user-id）。
			newServiceAwareUserIDInterceptor(healthServiceFullName),
			grpcinterceptor.NewServerTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(tracer)),
			grpcinterceptor.ServerLoggingInterceptor(),
			grpcinterceptor.ServerRecoveryInterceptor(),
		),
	}
	gs := grpc.NewServer(opts...)

	// 注册 service
	emotionquery.RegisterEmotionQueryServiceServer(gs, &emotionQueryServer{
		repo:              repo,
		fusedEmotionRepo:  fusedEmotionRepo,
		svcCtx:            svcCtx,
	})

	// 注册 health check（不带 user id 要求）
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("emotion.AI", healthpb.HealthCheckResponse_SERVING)
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
	logging.Printf("[grpc] services: EmotionQueryService (user id required)")

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
	repo             repository.EmotionRepo
	fusedEmotionRepo repository.FusedEmotionRepo
	// Sprint F2（2026-09-11）：加 svcCtx 以支持 MultiModalAnalyze/SynthesizeSpeech/AIHealth
	// 3 个业务 RPC（需要 logic 包内的 aiclient + svcCtx）。
	// 注：现有 4 RPC（Get*/Upsert*）不依赖 svcCtx，可继续 nil-safe。
	svcCtx *svc.ServiceContext
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

// GetFusedEmotion 实现 Stage 34 新 RPC：按 message_id 查多模态融合产物。
//
// 设计：
//   - fusedEmotionRepo == nil → Unimplemented（向后兼容现有 ai-svc 启动路径）
//   - message_id 无效 → InvalidArgument
//   - repo 返 nil → NotFound
//   - repo 返 error → Internal
//   - 成功 → 映射 model.FusedEmotion → proto.FusedEmotion
func (s *emotionQueryServer) GetFusedEmotion(ctx context.Context, req *emotionquery.GetFusedEmotionRequest) (*emotionquery.FusedEmotion, error) {
	if req.MessageId == 0 {
		return nil, status.Error(codes.InvalidArgument, "message_id is required")
	}
	if s.fusedEmotionRepo == nil {
		return nil, status.Error(codes.Unimplemented, "fused emotion not available on this server")
	}
	f, err := s.fusedEmotionRepo.GetByMessageID(ctx, req.MessageId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	if f == nil {
		return nil, status.Error(codes.NotFound, "fused emotion not found for this message")
	}
	return toProtoFusedEmotion(f), nil
}

// UpsertNeutralEmotion Stage 36-A3：chat-svc 在 Kafka 关闭（KAFKA_ENABLED=false /
// dev 模式）时，发消息成功后同步调用本 RPC 写入一条中性占位情绪，让前端"情绪分析"
// 模块在 dev 模式下也能立刻查到占位数据。
//
// 关键契约：
//   - event_id 非空 → 走幂等去重（DB UNIQUE on event_id）：同 event_id 二次调用
//     返回 was_inserted=false + 已存在行的 id（不再插入新行）。
//   - event_id 为空 → 直接插入（无去重）。生产路径下 event_id 必填（chat-svc 用
//     其 outbox UUID），空 event_id 仅用于诊断 / 单测。
//   - model 字段固定为 "sync-fallback"，便于前端 / 监控区分 Kafka 异步路径。
func (s *emotionQueryServer) UpsertNeutralEmotion(ctx context.Context, req *emotionquery.UpsertNeutralEmotionRequest) (*emotionquery.UpsertNeutralEmotionResponse, error) {
	if req.MessageId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "message_id is required and must be > 0")
	}
	if req.UserId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required and must be > 0")
	}
	if req.ConversationId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required and must be > 0")
	}
	if req.EventId == "" {
		return nil, status.Error(codes.InvalidArgument, "event_id is required (chat-svc outbox UUID) for idempotency")
	}

	// 幂等检查：先按 event_id 查一次
	if existing, err := s.repo.GetByEventID(ctx, req.EventId); err != nil {
		return nil, status.Errorf(codes.Internal, "lookup by event_id: %v", err)
	} else if existing != nil {
		return &emotionquery.UpsertNeutralEmotionResponse{
			EmotionAnalysisId: existing.ID,
			WasInserted:       false,
		}, nil
	}

	// 插入中性占位
	e := &model.EmotionAnalysis{
		MessageID:      req.MessageId,
		UserID:         req.UserId,
		ConversationID: req.ConversationId,
		EventID:        req.EventId,
		PrimaryEmotion: "neutral",
		SentimentScore: 0,
		Confidence:     0,
		Model:          "sync-fallback",
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, status.Errorf(codes.Internal, "create neutral placeholder: %v", err)
	}
	return &emotionquery.UpsertNeutralEmotionResponse{
		EmotionAnalysisId: e.ID,
		WasInserted:       true,
	}, nil
}

// =====================================================
// Sprint F2（2026-09-11）：3 业务 RPC 实现
// =====================================================

// MultiModalAnalyze 实现 MultiModalAnalyze RPC
//
// 行为契约：
//   - 复用 logic.NewMultiModalAnalyzeLogic（与 HTTP handler 同源，零行为变化）
//   - proto file_bytes (bytes) → logic.Analyze(kind, fileBytes, filename, text)
//   - 错误映射：
//     - aiclient.ErrNotConfigured / XTTSUnavailable → codes.Unavailable
//     - 其他 → codes.Internal
//
// 注意：本 RPC 当前只覆盖 persist=false 路径（与 chat 路径一致；persist=true
// 走 PersistMultiModalAnalyzeLogic，需要 svcCtx.Repo，本次 sprint 不扩）。
// 这是与 HTTP handler 行为差异，需在 chat 触发时扩。
func (s *emotionQueryServer) MultiModalAnalyze(ctx context.Context, req *emotionquery.MultiModalAnalyzeRequest) (*emotionquery.MultiModalAnalyzeResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "ai-svc service context not initialized")
	}
	if req.Kind == "" {
		return nil, status.Error(codes.InvalidArgument, "kind is required")
	}
	if req.Kind != "text" && len(req.FileBytes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "file_bytes is required for kind="+req.Kind)
	}

	// proto 中 text_content 是 string（空 = 无文本）；logic 也接受 string
	resp, err := logic.NewMultiModalAnalyzeLogic(s.svcCtx).Analyze(
		ctx, req.GetKind(), req.GetFileBytes(), req.GetFilename(), req.GetTextContent(),
	)
	if err != nil {
		return nil, mapAIError(err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "empty multi modal analyze response")
	}
	return &emotionquery.MultiModalAnalyzeResponse{
		Kind:           resp.Kind,
		Emotion:        resp.Emotion,
		Confidence:     resp.Confidence,
		SentimentScore: resp.Sentiment,
		Model:          resp.Model,
		Transcript:     resp.Transcript,
		AllScores:      resp.AllScores,
	}, nil
}

// SynthesizeSpeech 实现 SynthesizeSpeech RPC
//
// 复用 logic.NewSynthesizeSpeechLogic（与 HTTP /api/v1/tts/synthesize 同源）。
func (s *emotionQueryServer) SynthesizeSpeech(ctx context.Context, req *emotionquery.SynthesizeSpeechRequest) (*emotionquery.SynthesizeSpeechResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "ai-svc service context not initialized")
	}
	if req.GetText() == "" {
		return nil, status.Error(codes.InvalidArgument, "text is required")
	}
	resp, err := logic.NewSynthesizeSpeechLogic(s.svcCtx).Synthesize(
		ctx, req.GetText(), req.GetLanguage(), req.GetSpeed(),
	)
	if err != nil {
		return nil, mapAIError(err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "empty synthesize speech response")
	}
	return &emotionquery.SynthesizeSpeechResponse{
		Audio:      resp.Audio,
		SampleRate: int32(resp.SampleRate),
		Mime:       resp.MIME,
		Bytes:      int32(resp.Bytes),
		Text:       resp.Text,
		Language:   resp.Language,
	}, nil
}

// AIHealth 实现 AIHealth RPC
//
// 复用 logic.NewAIHealthLogic（与 HTTP /api/v1/ai/health 同源）。
func (s *emotionQueryServer) AIHealth(ctx context.Context, req *emotionquery.AIHealthRequest) (*emotionquery.AIHealthResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "ai-svc service context not initialized")
	}
	resp, err := logic.NewAIHealthLogic(s.svcCtx).Health(ctx)
	if err != nil {
		return nil, mapAIError(err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "empty ai health response")
	}
	out := &emotionquery.AIHealthResponse{
		TimeMs:     resp.Time,
		AllHealthy: resp.All,
	}
	if resp.FER != nil {
		out.Fer = &emotionquery.AIHealthEntry{
			Enabled: resp.FER.Enabled,
			Healthy: resp.FER.Healthy,
			Error:   resp.FER.Error,
		}
	}
	if resp.SV != nil {
		out.Sensevoice = &emotionquery.AIHealthEntry{
			Enabled: resp.SV.Enabled,
			Healthy: resp.SV.Healthy,
			Error:   resp.SV.Error,
		}
	}
	if resp.TTS != nil {
		out.Xtts = &emotionquery.AIHealthEntry{
			Enabled: resp.TTS.Enabled,
			Healthy: resp.TTS.Healthy,
			Error:   resp.TTS.Error,
		}
	}
	return out, nil
}

// mapAIError 把 ai-svc 业务错误映射到 gRPC status code
//
// 语义与 HTTP handler 一致：
//   - aiclient.ErrNotConfigured / logic.ErrXTTSUnavailable → codes.Unavailable（与 HTTP 503 对齐）
//   - 其他 → codes.Internal
//
// 不在此做 NotFound / InvalidArgument 区分——业务错误信息在 err.Error() 里。
func mapAIError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case errors.Is(err, aiclient.ErrNotConfigured),
		errors.Is(err, logic.ErrXTTSUnavailable),
		errors.Is(err, logic.ErrMultiModalNotInit),
		strings.Contains(msg, "call XTTS"),
		strings.Contains(msg, "XTTS_BASE_URL"):
		return status.Error(codes.Unavailable, msg)
	default:
		return status.Error(codes.Internal, msg)
	}
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

func toProtoFusedEmotion(f *model.FusedEmotion) *emotionquery.FusedEmotion {
	return &emotionquery.FusedEmotion{
		MessageId:           f.MessageID,
		UserId:              f.UserID,
		ConversationId:      f.ConversationID,
		PrimaryEmotion:      f.PrimaryEmotion,
		SentimentScore:      f.SentimentScore,
		Confidence:          f.Confidence,
		ModalityContrib:     f.ModalityContrib,
		Reasoning:           f.Reasoning,
		FusionMethod:        f.FusionMethod,
		AvailableModalities: f.AvailableModalities,
		CreatedAtMs:         f.CreatedAt.UnixMilli(),
	}
}
