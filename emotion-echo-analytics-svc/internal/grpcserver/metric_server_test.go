// Package grpcserver — metric_server_test.go
//
// Stage 62 PR-3.2: analytics-svc gRPC server 实现单元测试（仿 user_server_test.go）
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
)

func startAnalyticsTestServer(t *testing.T, svcCtxs ...*svc.ServiceContext) (*Server, *grpc.ClientConn, func()) {
	t.Helper()
	var svcCtx *svc.ServiceContext
	if len(svcCtxs) > 0 {
		svcCtx = svcCtxs[0]
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	srv := New(svcCtx, port)
	require.NotNil(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Start(ctx) }()

	addr := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr = srv.Addr()
		if addr != "" && addr != fmt.Sprintf(":%d", port) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		cancel()
		select {
		case <-serveErr:
		case <-time.After(2 * time.Second):
		}
	}
	return srv, conn, cleanup
}

// ============ Unimplemented 阶段行为契约 ============

// TestAnalyticsServer_ReportsTrend_IntentDistribution_DeterministicOrder 锁定
// Stage 85 契约：ReportsTrend 响应携带区间意图分布（msg_summary_v 聚合），且
// 按 6 类白名单确定性顺序输出（与 ReportsDaily 同序），便于前端饼图稳定渲染。
func TestAnalyticsServer_ReportsTrend_IntentDistribution_DeterministicOrder(t *testing.T) {
	reportRepo := repository.NewInMemoryReportRepo()
	reportRepo.SetTrend(&repository.TrendReport{
		UserID:    7,
		Type:      "weekly",
		StartDate: "2026-09-01",
		EndDate:   "2026-09-07",
		Points: []repository.TrendPoint{
			{Date: "2026-09-01", PrimaryEmotion: "happy", Count: 3},
		},
		// 故意乱序注入，验证输出仍按白名单顺序
		IntentCounts: map[string]int64{
			"tech_help":        2,
			"lifestyle":        1,
			"emotional_support": 3,
			"unknown_intent":   99, // 白名单外不输出
		},
	}, nil)
	svcCtx := svc.NewServiceContextWithReports(config.Config{}, repository.NewInMemoryEventRepo(), reportRepo)

	_, conn, cleanup := startAnalyticsTestServer(t, svcCtx)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	// 服务有 user-id 拦截器，须带 x-user-id metadata（同其他测试）
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-user-id", "7"))
	resp, err := client.ReportsTrend(ctx, &emotionanalytics.ReportsTrendRequest{
		UserId: 7,
		Type:   "weekly",
		DateRange: &emotionanalytics.DateRange{
			StartDate: parseDateProto("2026-09-01"),
			EndDate:   parseDateProto("2026-09-07"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	got := make([]string, 0, len(resp.IntentDistribution))
	counts := make(map[string]int32, len(resp.IntentDistribution))
	for _, ic := range resp.IntentDistribution {
		got = append(got, ic.Intent)
		counts[ic.Intent] = ic.Count
	}
	assert.Equal(t, []string{"emotional_support", "tech_help", "lifestyle"}, got,
		"按 6 类白名单相对顺序输出，白名单外（unknown_intent）不出现")
	assert.Equal(t, int32(3), counts["emotional_support"])
	assert.Equal(t, int32(2), counts["tech_help"])
	assert.Equal(t, int32(1), counts["lifestyle"])
}

func TestAnalyticsServer_ReportsDaily_MissingUserID_ReturnsNonOK(t *testing.T) {
	_, conn, cleanup := startAnalyticsTestServer(t)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	_, err := client.ReportsDaily(context.Background(), &emotionanalytics.ReportsDailyRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.NotEqual(t, "OK", st.Code().String(), "期望被拦截（非 OK）")
}

func TestAnalyticsServer_ReportsTrend_WithUserID_NoPanic(t *testing.T) {
	_, conn, cleanup := startAnalyticsTestServer(t)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	_, err := client.ReportsTrend(ctx, &emotionanalytics.ReportsTrendRequest{
		UserId: 7,
		Type:   "weekly",
	})
	if err == nil {
		t.Fatal("PR-3.2 阶段 ReportsTrend 应返 Unavailable（svcCtx 未注入）")
	}
	st, _ := status.FromError(err)
	assert.Equal(t, "Unavailable", st.Code().String(), "PR-3.2 阶段期望 Unavailable")
}

func TestAnalyticsServer_UserBehaviorDayNight_NoPanic(t *testing.T) {
	_, conn, cleanup := startAnalyticsTestServer(t)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	_, err := client.UserBehaviorDayNight(ctx, &emotionanalytics.UserBehaviorRequest{
		UserId: 7,
	})
	if err == nil {
		t.Fatal("PR-3.2 阶段 UserBehaviorDayNight 应返 Unavailable")
	}
	st, _ := status.FromError(err)
	t.Logf("UserBehaviorDayNight code=%s msg=%s", st.Code(), st.Message())
}

func TestAnalyticsServer_MentalHealthTrigger_RequiresUserID(t *testing.T) {
	_, conn, cleanup := startAnalyticsTestServer(t)
	defer cleanup()

	client := emotionanalytics.NewAnalyticsServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	_, err := client.MentalHealthTrigger(ctx, &emotionanalytics.MentalHealthTriggerRequest{
		UserId:        7,
		TriggerReason: "manual",
	})
	if err == nil {
		t.Fatal("PR-3.2 阶段 MentalHealthTrigger 应返 Unavailable")
	}
	st, _ := status.FromError(err)
	assert.Equal(t, "Unavailable", st.Code().String())
}

// ============ 单元方法：type / New 存在性 ============

func TestAnalyticsServer_TypeImplement(t *testing.T) {
	var _ emotionanalytics.AnalyticsServiceServer = (*analyticsServer)(nil)
}

func TestAnalyticsServer_New_ReturnsNonNil(t *testing.T) {
	srv := New(nil, 8885)
	require.NotNil(t, srv)
	assert.Equal(t, 8885, srv.port)
}

func TestAnalyticsServer_PortPropagated(t *testing.T) {
	srv := New(nil, 9093)
	assert.Equal(t, 9093, srv.port)
}

// ============ Message types ============

func TestAnalyticsServer_MessageTypesExist(t *testing.T) {
	assert.NotNil(t, &emotionanalytics.ReportsDailyResponse{})
	assert.NotNil(t, &emotionanalytics.UserBehaviorRequest{})
	assert.NotNil(t, &emotionanalytics.MentalHealthTrendResponse{})
}