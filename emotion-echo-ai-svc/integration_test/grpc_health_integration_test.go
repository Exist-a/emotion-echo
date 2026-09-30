//go:build integration
// +build integration

// Package integration_test 跑真实 Postgres + ai-svc emotionrepo + emotionquery proto 接口的端到端集成测试。
//
// 流程：
//   - testcontainers 起 postgres:15-alpine
//   - emotion_echo_ai schema + emotion_analysis 表
//   - PostgresEmotionRepo 注入数据
//   - 启真实 grpc.Server（注册 EmotionQueryService + 标准 grpc.health.v1 health）
//   - 用 grpc.ClientConn + emotionhealth.Client + emotionquery.NewEmotionQueryServiceClient 真实交互
//
// 跑：  go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"net"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	emotionhealth "github.com/emotion-echo/shared/pkg/healthcheck"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// pgContainerDesc 保留旧名，改为走共享 fixture（E2E-F-168）。
//
// 旧实现在这里**手写了一份只建 emotion_analysis 的 DDL**（带一条非 partial 的
// `CONSTRAINT uq_emotion_analysis_event_id UNIQUE`），与生产的真实 schema 不一致 ——
// 生产上 i006/i009 把索引改成了 partial 形式，仓储的 `ON CONFLICT (event_id)`
// 也必须带谓词才能匹配。用真实 schema 跑，才能测出这类"测试库与生产不一致导致
// 假绿/假红"的问题。
// grpcServerHealthSvcName 与生产构造函数注册的 per-service 名保持一致（proto 真名）。
const grpcServerHealthSvcName = "emotion_ai.v1.EmotionQueryService"

func pgContainerDesc(t *testing.T, _ context.Context) (*gorm.DB, func()) {
	t.Helper()
	return newAIDB(t)
}

// startFakeAIGRPCServer 起真实 grpc.Server，注册 health + emotionquery.EmotionQueryService
// 返回 listener（用于关停）+ server 本身。
func startFakeAIGRPCServer(t *testing.T, repo repository.EmotionRepo) (*grpc.Server, net.Listener) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	gs := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcinterceptor.ServerLoggingInterceptor(),
			grpcinterceptor.ServerRecoveryInterceptor(),
		),
	)
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus(grpcServerHealthSvcName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, healthSrv)
	emotionquery.RegisterEmotionQueryServiceServer(gs, &emotionQueryAdapter{repo: repo})

	go func() { _ = gs.Serve(lis) }()
	return gs, lis
}

func dialGRPC(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err)
	return conn
}

// ---------- 适配器实现 EmotionQueryServiceServer 接口 ----------

type emotionQueryAdapter struct {
	emotionquery.UnimplementedEmotionQueryServiceServer
	repo repository.EmotionRepo
}

func (s *emotionQueryAdapter) GetEmotionByMessage(ctx context.Context, req *emotionquery.GetEmotionByMessageRequest) (*emotionquery.Emotion, error) {
	if req.MessageId == 0 {
		return nil, status.Error(codes.InvalidArgument, "message_id required")
	}
	e, err := s.repo.GetByMessageID(ctx, req.MessageId)
	if err != nil || e == nil {
		return nil, status.Error(codes.NotFound, "emotion not found")
	}
	return toProtoEmotion(e), nil
}

func (s *emotionQueryAdapter) GetEmotionByConversation(ctx context.Context, req *emotionquery.GetEmotionByConversationRequest) (*emotionquery.EmotionList, error) {
	limit := int(req.Limit)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.repo.ListByConversationID(ctx, req.ConversationId)
	if err != nil {
		return nil, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]*emotionquery.Emotion, 0, len(rows))
	for i := range rows {
		out = append(out, toProtoEmotion(&rows[i]))
	}
	return &emotionquery.EmotionList{Items: out, Total: int32(len(out))}, nil
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

// ---------- 测试 ----------

// TestAIGRPC_HealthCheckIntegration 起真实 Postgres + 起真实 grpc.Server，
// 用 emotionhealth.Client 检查 SERVING / WaitForReady / 不存在 service
func TestAIGRPC_HealthCheckIntegration(t *testing.T) {
	ctx := context.Background()

	db, cleanup := pgContainerDesc(t, ctx)
	t.Cleanup(cleanup)

	repo := repository.NewPostgresEmotionRepo(db)

	gs, lis := startFakeAIGRPCServer(t, repo)
	t.Cleanup(func() { gs.GracefulStop(); _ = lis.Close() })

	conn := dialGRPC(t, lis.Addr().String())
	t.Cleanup(func() { _ = conn.Close() })

	hc := emotionhealth.NewClient(conn)

	// 1. Check(真实 service 名) → SERVING
	st, err := hc.Check(ctx, grpcServerHealthSvcName)
	require.NoError(t, err)
	require.Equal(t, emotionhealth.ServingStatusServing, st)

	// 2. Check("") — service liveness（grpc 规范的空服务名表示整 server）
	st, err = hc.Check(ctx, "")
	require.NoError(t, err)
	require.Equal(t, emotionhealth.ServingStatusServing, st)

	// 3. WaitForReady
	require.NoError(t, hc.WaitForReady(ctx, grpcServerHealthSvcName, 2*time.Second))

	// 4. Check 不存在的 service — grpc health 对 unknown service 返 NotFound error
	// shared emotionhealth.Client.Check 已 normalizes error 为 ServiceUnknown
	st, err = hc.Check(ctx, "not-found")
	// shared 会将 err 转为 nil 后返 ServiceUnknown（注释里说明）
	require.Equal(t, emotionhealth.ServingStatusServiceUnknown, st)
	_ = err
}

// TestAIGRPC_EmotionQueryIntegration 真实 Postgres 注入数据 → gRPC 查回
func TestAIGRPC_EmotionQueryIntegration(t *testing.T) {
	ctx := context.Background()

	db, cleanup := pgContainerDesc(t, ctx)
	t.Cleanup(cleanup)

	repo := repository.NewPostgresEmotionRepo(db)

	// 写测试数据
	e := &model.EmotionAnalysis{
		MessageID:      99,
		UserID:         7,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.65,
		Confidence:     0.8,
		Model:          "keyword-stub-v1",
	}
	require.NoError(t, repo.Create(ctx, e))

	gs, lis := startFakeAIGRPCServer(t, repo)
	t.Cleanup(func() { gs.GracefulStop(); _ = lis.Close() })

	conn := dialGRPC(t, lis.Addr().String())
	t.Cleanup(func() { _ = conn.Close() })

	emotionCli := emotionquery.NewEmotionQueryServiceClient(conn)

	// 1. GetEmotionByMessage — 命中
	resp, err := emotionCli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 99})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "happy", resp.PrimaryEmotion)
	require.Equal(t, int64(99), resp.MessageId)
	require.InDelta(t, 0.65, resp.SentimentScore, 0.001)
	require.InDelta(t, 0.8, resp.Confidence, 0.001)
	require.Equal(t, "keyword-stub-v1", resp.Model)

	// 2. GetEmotionByMessage — 不存在
	_, err = emotionCli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 9999})
	require.Error(t, err)

	// 3. GetEmotionByConversation — 命中
	list, err := emotionCli.GetEmotionByConversation(ctx, &emotionquery.GetEmotionByConversationRequest{
		ConversationId: 50,
		Limit:          10,
	})
	require.NoError(t, err)
	require.NotNil(t, list)
	require.GreaterOrEqual(t, len(list.Items), 1)
	require.Equal(t, int32(len(list.Items)), list.Total)

	// 4. GetEmotionByConversation — 不存在 conv
	list2, err := emotionCli.GetEmotionByConversation(ctx, &emotionquery.GetEmotionByConversationRequest{
		ConversationId: 999999,
		Limit:          10,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(list2.Items))
	require.Equal(t, int32(0), list2.Total)
}

// TestAIGRPC_PostgresDown_HealthCheckIntegration 关掉容器后，
// 真实 PostgresEmotionRepo 的查询应该返错而不 panic（grpcdown 后 client 应收 NOT_FOUND / INTERNAL）
func TestAIGRPC_PostgresDown_EmotionQueryError(t *testing.T) {
	ctx := context.Background()

	// 这个用例要主动终止容器来验"PG 宕机"路径，所以拿容器句柄而非只拿 cleanup
	db, pgC, _ := newAIDBWithContainer(t)
	repo := repository.NewPostgresEmotionRepo(db)

	gs, lis := startFakeAIGRPCServer(t, repo)

	// 起阶段
	conn := dialGRPC(t, lis.Addr().String())
	emotionCli := emotionquery.NewEmotionQueryServiceClient(conn)
	_, _ = emotionCli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 99})

	// 停 Postgres
	require.NoError(t, pgC.Terminate(ctx))

	// 给 server 一点时间感知连接断开
	time.Sleep(200 * time.Millisecond)

	// 查询应返 err，不 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("gRPC client should not panic, got %v", r)
		}
	}()
	_, err := emotionCli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 99})
	require.Error(t, err)

	// 清理
	gs.GracefulStop()
	_ = lis.Close()
	_ = conn.Close()
}
