// Package grpcserver — metric_server_fused_test.go
//
// CL-07（后端切片）Phase 3 · RED：ReportsDaily RPC 必须把 DailyReport 的
// FusedEmotionDistribution 映射到 proto ReportsDailyResponse 的新字段。
//
// 契约：fused 分布 / 融合方法占比 / 多模态占比 都跨 gRPC 边界到 BFF。
// 无融合数据时这些字段为空（不报错）。

package grpcserver

import (
	"context"
	"testing"
	"time"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
)

func TestAnalyticsServer_ReportsDaily_FusedDistribution(t *testing.T) {
	reportRepo := repository.NewInMemoryReportRepo()
	reportRepo.SetDaily(&repository.DailyReport{
		UserID:        7,
		Date:          "2026-10-16",
		EmotionCounts: map[string]int64{"happy": 3},
	}, nil)

	fusedRepo := repository.NewInMemoryFusedReportRepo()
	fusedRepo.SetDaily(7, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), &repository.FusedEmotionDistribution{
		ByEmotion:  map[string]int64{"happy": 5, "sad": 2},
		ByMethod:   map[string]int64{"llm": 4, "late_fusion_weighted": 3},
		FaceRatio:  0.5,
		VoiceRatio: 0.25,
		Total:      7,
	})

	svcCtx := svc.NewServiceContextWithReports(config.Config{}, repository.NewInMemoryEventRepo(), reportRepo)
	svcCtx.FusedReportRepo = fusedRepo

	_, conn, cleanup := startAnalyticsTestServer(t, svcCtx)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-user-id", "7"))
	resp, err := client.ReportsDaily(ctx, &emotionanalytics.ReportsDailyRequest{
		UserId: 7,
		Date:   parseDateProto("2026-10-16"),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// 融合情绪分布
	fusedByEmotion := map[string]int32{}
	for _, e := range resp.FusedEmotionDistribution {
		fusedByEmotion[e.Emotion] = e.Count
	}
	assert.Equal(t, int32(5), fusedByEmotion["happy"], "fused 分布须跨 gRPC 边界")
	assert.Equal(t, int32(2), fusedByEmotion["sad"])

	// 融合方法占比
	fusedByMethod := map[string]int32{}
	for _, m := range resp.FusionMethodBreakdown {
		fusedByMethod[m.Method] = m.Count
	}
	assert.Equal(t, int32(4), fusedByMethod["llm"])
	assert.Equal(t, int32(3), fusedByMethod["late_fusion_weighted"])

	// 多模态占比
	assert.InDelta(t, 0.5, resp.FusedFaceRatio, 1e-9)
	assert.InDelta(t, 0.25, resp.FusedVoiceRatio, 1e-9)
}

func TestAnalyticsServer_ReportsDaily_NoFusedData_EmptyNotError(t *testing.T) {
	reportRepo := repository.NewInMemoryReportRepo()
	reportRepo.SetDaily(&repository.DailyReport{UserID: 7, Date: "2026-10-16", EmotionCounts: map[string]int64{}}, nil)
	// FusedReportRepo 未接线 → 无融合数据
	svcCtx := svc.NewServiceContextWithReports(config.Config{}, repository.NewInMemoryEventRepo(), reportRepo)

	_, conn, cleanup := startAnalyticsTestServer(t, svcCtx)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-user-id", "7"))
	resp, err := client.ReportsDaily(ctx, &emotionanalytics.ReportsDailyRequest{
		UserId: 7,
		Date:   parseDateProto("2026-10-16"),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.FusedEmotionDistribution, "无融合数据 → 空，不报错")
	assert.Empty(t, resp.FusionMethodBreakdown)
}
