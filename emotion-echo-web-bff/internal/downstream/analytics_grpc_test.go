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
	"google.golang.org/grpc/metadata"
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

func startMockAnalyticsBufConn(t *testing.T, mock emotionanalytics.AnalyticsServiceServer) (*grpc.ClientConn, func()) {
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

// =====================================================
// Stage 112 修复（Bug E）：user-behavior / mental-health 三个 RPC 必须注入
// x-user-id metadata
//
// 背景：analytics-svc 的 gRPC 拦截器要求所有 RPC 带 x-user-id metadata。
// BFF downstream 的 InteractionDepth / FrequencyTrend / MentalAssessment
// 三个方法漏包 withUserID(ctx) → analytics 返 "unauthenticated: missing
// x-user-id metadata" → BFF 透传 401 → 前端 useApi clearAuth + 跳 /login。
// 浏览器实测："我的空间"页（onMounted 调 3 个 user-behavior 端点）被踢回 /login。
// =====================================================

// metadataCapturingServer 记录收到的 x-user-id metadata
type metadataCapturingServer struct {
	emotionanalytics.UnimplementedAnalyticsServiceServer
	gotUserID string
}

func (m *metadataCapturingServer) capture(ctx context.Context) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-user-id"); len(v) > 0 {
			m.gotUserID = v[0]
		}
	}
}

func (m *metadataCapturingServer) UserBehaviorDepth(ctx context.Context, _ *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorDepthResponse, error) {
	m.capture(ctx)
	return &emotionanalytics.UserBehaviorDepthResponse{}, nil
}

func (m *metadataCapturingServer) UserBehaviorFrequency(ctx context.Context, _ *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorFrequencyResponse, error) {
	m.capture(ctx)
	return &emotionanalytics.UserBehaviorFrequencyResponse{}, nil
}

func (m *metadataCapturingServer) MentalHealthAssessment(ctx context.Context, _ *emotionanalytics.MentalHealthAssessmentRequest) (*emotionanalytics.MentalHealthAssessmentResponse, error) {
	m.capture(ctx)
	return &emotionanalytics.MentalHealthAssessmentResponse{}, nil
}

func TestAnalyticsGRPCClient_InteractionDepth_InjectsUserIDMetadata(t *testing.T) {
	mock := &metadataCapturingServer{}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	// 模拟 handler 传入的 ctx：session.WithRequestAuth 已注入 user_id
	ctx := WithUserID(context.Background(), 7)
	_, err := client.InteractionDepth(ctx, 7, "2026-09-01", "2026-09-07")
	require.NoError(t, err)
	assert.Equal(t, "7", mock.gotUserID, "InteractionDepth 必须注入 x-user-id metadata（否则 analytics-svc 401）")
}

func TestAnalyticsGRPCClient_FrequencyTrend_InjectsUserIDMetadata(t *testing.T) {
	mock := &metadataCapturingServer{}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	ctx := WithUserID(context.Background(), 7)
	_, err := client.FrequencyTrend(ctx, 7, "2026-09-01", "2026-09-07")
	require.NoError(t, err)
	assert.Equal(t, "7", mock.gotUserID, "FrequencyTrend 必须注入 x-user-id metadata")
}

// =====================================================
// E2E-11：UserBehaviorDepth 的 4 个指标必须从 proto Buckets 完整还原
//
// 背景：proto UserBehaviorDepthResponse 只有 `repeated ChartDataPoint buckets`
// + `double average_length` 两个字段，承载不下 InteractionDepth 的 4 个指标。
// analytics-svc 侧（metric_server.go）把 4 个指标编码进 buckets 的 Label；
// BFF gRPC 客户端必须按 Label 还原，否则 totalMessages / totalConversations /
// longestConversationMs 恒为 0。
//
// 浏览器实测（2026-09-19）：深度端点返回
//   {"avgSessionRounds":3.41,"totalConversations":0,"totalMessages":0,...}
// ——avgSessionRounds 有值（读的是 average_length），其余 3 项为 0（未读 buckets）。
// =====================================================

// depthBucketsServer 返回带 4 个 Label 的 buckets（与 analytics-svc 编码一致）
type depthBucketsServer struct {
	emotionanalytics.UnimplementedAnalyticsServiceServer
}

func (d *depthBucketsServer) UserBehaviorDepth(_ context.Context, _ *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorDepthResponse, error) {
	return &emotionanalytics.UserBehaviorDepthResponse{
		Buckets: []*emotionanalytics.ChartDataPoint{
			{Timestamp: 1, Value: 150, Label: "totalMessages"},
			{Timestamp: 2, Value: 12, Label: "totalConversations"},
			{Timestamp: 3, Value: 12.5, Label: "avgMessagesPerConv"},
			{Timestamp: 4, Value: 340000, Label: "longestConversationMs"},
		},
		AverageLength: 12.5,
	}, nil
}

func TestAnalyticsGRPCClient_InteractionDepth_MapsAllMetricsFromBuckets(t *testing.T) {
	conn, cleanup := startMockAnalyticsBufConn(t, &depthBucketsServer{})
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	ctx := WithUserID(context.Background(), 7)
	depth, err := client.InteractionDepth(ctx, 7, "2026-09-01", "2026-09-07")
	require.NoError(t, err)
	require.NotNil(t, depth)

	assert.Equal(t, int64(150), depth.TotalMessages,
		"E2E-11: totalMessages 必须从 buckets[label=totalMessages] 还原（否则页面柱状图恒 0）")
	assert.Equal(t, int64(12), depth.TotalConversations,
		"E2E-11: totalConversations 必须从 buckets 还原")
	assert.Equal(t, 12.5, depth.AvgMessagesPerConv,
		"E2E-11: avgMessagesPerConv 必须从 buckets/AverageLength 还原")
	assert.Equal(t, int64(340000), depth.LongestConversationMs,
		"E2E-11: longestConversationMs 必须从 buckets 还原")
}

func TestAnalyticsGRPCClient_MentalAssessment_InjectsUserIDMetadata(t *testing.T) {
	mock := &metadataCapturingServer{}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	ctx := WithUserID(context.Background(), 7)
	_, err := client.MentalAssessment(ctx, 7, "phq9")
	require.NoError(t, err)
	assert.Equal(t, "7", mock.gotUserID, "MentalAssessment 必须注入 x-user-id metadata")
}
