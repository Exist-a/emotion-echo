// Package downstream — analytics_grpc_test.go
//
// Stage 85: BFF → analytics-svc gRPC client 单元测试（仿 chat_grpc_test.go）
//
// 行为契约：
//   - TrendReport 调 ReportsTrend RPC，把响应 intent_distribution 映射进
//     TrendReport.IntentCounts（Stage 85 趋势报告意图维度——防"第 N 处丢字段"）
//
// 测试策略：bufconn mock analytics server + 真实 grpc.ClientConn + 真实客户端。
package downstream

import (
	"context"
	"net"
	"testing"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// mockAnalyticsServer 在 bufconn 上提供 AnalyticsService 实现（仅本文件用到的 RPC）
type mockAnalyticsServer struct {
	emotionanalytics.UnimplementedAnalyticsServiceServer

	trendResp *emotionanalytics.ReportsTrendResponse
}

func (m *mockAnalyticsServer) ReportsTrend(_ context.Context, _ *emotionanalytics.ReportsTrendRequest) (*emotionanalytics.ReportsTrendResponse, error) {
	return m.trendResp, nil
}

func startMockAnalyticsBufConn(t *testing.T, mock *mockAnalyticsServer) (*grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	emotionanalytics.RegisterAnalyticsServiceServer(srv, mock)
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("mock server stopped: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return conn, cleanup
}

// TestAnalyticsGRPCClient_TrendReport_MapsIntentDistribution 锁定 proto
// intent_distribution → downstream.TrendReport.IntentCounts 的映射不丢字段。
func TestAnalyticsGRPCClient_TrendReport_MapsIntentDistribution(t *testing.T) {
	mock := &mockAnalyticsServer{trendResp: &emotionanalytics.ReportsTrendResponse{
		Type: "weekly",
		DataPoints: []*emotionanalytics.ChartDataPoint{
			{Timestamp: parseDate("2026-09-01"), Label: "happy", Value: 3},
		},
		IntentDistribution: []*emotionanalytics.IntentCount{
			{Intent: "emotional_support", Count: 3},
			{Intent: "tech_help", Count: 2},
		},
	}}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	report, err := client.TrendReport(context.Background(), 42, "weekly", "2026-09-01", "2026-09-07")
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, "weekly", report.Type)
	require.Len(t, report.Points, 1)
	assert.Equal(t, map[string]int64{"emotional_support": 3, "tech_help": 2}, report.IntentCounts)
}

// TestAnalyticsGRPCClient_TrendReport_NilIntentDistribution 空分布映射为空 map
// （非 nil，便于 view 层 len 判断统一）。
func TestAnalyticsGRPCClient_TrendReport_NilIntentDistribution(t *testing.T) {
	mock := &mockAnalyticsServer{trendResp: &emotionanalytics.ReportsTrendResponse{
		Type: "weekly",
	}}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	report, err := client.TrendReport(context.Background(), 42, "weekly", "2026-09-01", "2026-09-07")
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Empty(t, report.IntentCounts)
}
