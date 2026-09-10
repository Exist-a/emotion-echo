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
)

func startAnalyticsTestServer(t *testing.T) (*Server, *grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	srv := New(nil, port)
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