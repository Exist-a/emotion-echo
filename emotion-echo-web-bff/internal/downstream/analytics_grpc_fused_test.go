// Package downstream — analytics_grpc_fused_test.go
//
// CL-07（后端切片）Phase 3 · RED：BFF gRPC 客户端必须把 ReportsDailyResponse
// 的融合字段映射进 downstream.DailyReport（防"第 N 处丢字段"）。
//
// 复用 analytics_grpc_test.go 的 bufconn mock（已扩展 ReportsDaily）。

package downstream

import (
	"context"
	"testing"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsGRPCClient_DailyReport_MapsFusedDistribution(t *testing.T) {
	mock := &mockAnalyticsServer{
		dailyResp: &emotionanalytics.ReportsDailyResponse{
			MessageCount:      10,
			ConversationCount: 2,
			EmotionDistribution: []*emotionanalytics.EmotionDistribution{
				{Emotion: "happy", Count: 3},
			},
			FusedEmotionDistribution: []*emotionanalytics.EmotionDistribution{
				{Emotion: "happy", Count: 5},
				{Emotion: "sad", Count: 2},
			},
			FusionMethodBreakdown: []*emotionanalytics.FusionMethodBreakdown{
				{Method: "llm", Count: 4},
				{Method: "late_fusion_weighted", Count: 3},
			},
			FusedFaceRatio:  0.5,
			FusedVoiceRatio: 0.25,
		},
	}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	client := NewAnalyticsGRPCClient(conn)
	got, err := client.DailyReport(context.Background(), 7, "2026-10-16")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.FusedEmotionDistribution, "融合字段必须跨 gRPC 映射进 BFF DTO")
	assert.Equal(t, int64(5), got.FusedEmotionDistribution.ByEmotion["happy"])
	assert.Equal(t, int64(2), got.FusedEmotionDistribution.ByEmotion["sad"])
	assert.Equal(t, int64(4), got.FusedEmotionDistribution.ByMethod["llm"])
	assert.Equal(t, int64(3), got.FusedEmotionDistribution.ByMethod["late_fusion_weighted"])
	assert.InDelta(t, 0.5, got.FusedEmotionDistribution.FaceRatio, 1e-9)
	assert.InDelta(t, 0.25, got.FusedEmotionDistribution.VoiceRatio, 1e-9)
}

func TestAnalyticsGRPCClient_DailyReport_NoFused_NilField(t *testing.T) {
	mock := &mockAnalyticsServer{
		dailyResp: &emotionanalytics.ReportsDailyResponse{MessageCount: 1},
	}
	conn, cleanup := startMockAnalyticsBufConn(t, mock)
	defer cleanup()

	got, err := NewAnalyticsGRPCClient(conn).DailyReport(context.Background(), 7, "2026-10-16")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.FusedEmotionDistribution, "无融合数据 → nil（不造空结构）")
}
