// Package grpcserver — metric_server.go
//
// Stage 62 PR-3.2: 实现 AnalyticsServiceServer interface（9 RPC）
//
// PR-3.2 阶段：所有方法占位返 Unimplemented（PR-3.3 阶段补完）：
//   - ReportsDaily / ReportsTrend
//   - UserBehaviorDayNight / Depth / Frequency
//   - MentalHealthAssessment / History / Trigger / Trend

package grpcserver

import (
	"context"

	"emotion-echo-analytics-svc/internal/svc"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// analyticsServer 实现 emotionanalytics.AnalyticsServiceServer
type analyticsServer struct {
	emotionanalytics.UnimplementedAnalyticsServiceServer
	svcCtx *svc.ServiceContext
}

// ReportsDaily 占位
func (s *analyticsServer) ReportsDaily(ctx context.Context, req *emotionanalytics.ReportsDailyRequest) (*emotionanalytics.ReportsDailyResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "ReportsDaily: PR-3.3 阶段补完")
}

// ReportsTrend 占位
func (s *analyticsServer) ReportsTrend(ctx context.Context, req *emotionanalytics.ReportsTrendRequest) (*emotionanalytics.ReportsTrendResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "ReportsTrend: PR-3.3 阶段补完")
}

// UserBehaviorDayNight 占位
func (s *analyticsServer) UserBehaviorDayNight(ctx context.Context, req *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorDayNightResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "UserBehaviorDayNight: PR-3.3 阶段补完")
}

// UserBehaviorDepth 占位
func (s *analyticsServer) UserBehaviorDepth(ctx context.Context, req *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorDepthResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "UserBehaviorDepth: PR-3.3 阶段补完")
}

// UserBehaviorFrequency 占位
func (s *analyticsServer) UserBehaviorFrequency(ctx context.Context, req *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorFrequencyResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "UserBehaviorFrequency: PR-3.3 阶段补完")
}

// MentalHealthAssessment 占位
func (s *analyticsServer) MentalHealthAssessment(ctx context.Context, req *emotionanalytics.MentalHealthAssessmentRequest) (*emotionanalytics.MentalHealthAssessmentResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "MentalHealthAssessment: PR-3.3 阶段补完")
}

// MentalHealthHistory 占位
func (s *analyticsServer) MentalHealthHistory(ctx context.Context, req *emotionanalytics.MentalHealthHistoryRequest) (*emotionanalytics.MentalHealthHistoryResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "MentalHealthHistory: PR-3.3 阶段补完")
}

// MentalHealthTrigger 占位
func (s *analyticsServer) MentalHealthTrigger(ctx context.Context, req *emotionanalytics.MentalHealthTriggerRequest) (*emotionanalytics.MentalHealthTriggerResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "MentalHealthTrigger: PR-3.3 阶段补完")
}

// MentalHealthTrend 占位
func (s *analyticsServer) MentalHealthTrend(ctx context.Context, req *emotionanalytics.MentalHealthTrendRequest) (*emotionanalytics.MentalHealthTrendResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "MentalHealthTrend: PR-3.3 阶段补完")
}