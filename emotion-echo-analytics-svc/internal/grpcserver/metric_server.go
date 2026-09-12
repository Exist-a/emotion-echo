// Package grpcserver — metric_server.go
//
// Stage 62 PR-3.3: 实现 AnalyticsServiceServer interface（HTTP 与 gRPC 共享 logic 层）
//
// 9 个 rpc 方法对应 analytics-svc 已有的 logic 层
//
// 设计：每个 RPC 调对应 logic.*Logic 方法；proto ↔ types 转换用 helper 函数

package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"emotion-echo-analytics-svc/internal/logic"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	grpcerr "github.com/emotion-echo/shared/pkg/grpcerr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// analyticsServer 实现 emotionanalytics.AnalyticsServiceServer
type analyticsServer struct {
	emotionanalytics.UnimplementedAnalyticsServiceServer
	svcCtx *svc.ServiceContext
}

// B4: 注册业务 sentinel errors
func init() {
	grpcerr.MapError(repository.ErrNotFound, codes.NotFound)
}

// parseDate YYYY-MM-DD → unix seconds（start of day UTC）
func parseDateProto(s string) int64 {
	if len(s) < 10 {
		return 0
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return 0
	}
	return t.Unix()
}

// unixToDate unix seconds → "YYYY-MM-DD"
func unixToDateProto(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

// toProtoChartDataPoints []TrendPoint → []*emotionanalytics.ChartDataPoint
func toProtoChartDataPoints(points []types.TrendPoint) []*emotionanalytics.ChartDataPoint {
	out := make([]*emotionanalytics.ChartDataPoint, 0, len(points))
	for _, p := range points {
		out = append(out, &emotionanalytics.ChartDataPoint{
			Timestamp: parseDateProto(p.Date),
			Value:     p.AvgSentiment,
			Label:     p.PrimaryEmotion,
		})
	}
	return out
}

// toProtoEmotionDistribution []EmotionCount → []*emotionanalytics.EmotionDistribution
func toProtoEmotionDistribution(counts map[string]int64, total int64) []*emotionanalytics.EmotionDistribution {
	out := make([]*emotionanalytics.EmotionDistribution, 0, len(counts))
	for emotion, count := range counts {
		var pct float64
		if total > 0 {
			pct = float64(count) / float64(total) * 100
		}
		out = append(out, &emotionanalytics.EmotionDistribution{
			Emotion:    emotion,
			Count:      int32(count),
			Percentage: pct,
		})
	}
	return out
}

// ReportsDaily 实现 ReportsDaily RPC
func (s *analyticsServer) ReportsDaily(ctx context.Context, req *emotionanalytics.ReportsDailyRequest) (*emotionanalytics.ReportsDailyResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	dateStr := unixToDateProto(req.Date)
	if dateStr == "" {
		dateStr = time.Now().UTC().Format("2006-01-02")
	}
	resp, err := logic.NewReportsDailyLogic(ctx, s.svcCtx).GetDailyReport(&types.GetDailyReportReq{
		UserID: req.UserId,
		Date:   dateStr,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// 没数据 → 返空响应（不是 error）
			return &emotionanalytics.ReportsDailyResponse{
				Date: req.Date,
			}, nil
		}
		return nil, grpcerr.MapToError(err, "reportsDaily")
	}
	if resp == nil || resp.Report == nil {
		return &emotionanalytics.ReportsDailyResponse{Date: req.Date}, nil
	}
	r := resp.Report
	totalEmotions := int64(0)
	for _, c := range r.EmotionCounts {
		totalEmotions += c
	}
	return &emotionanalytics.ReportsDailyResponse{
		Summary:            fmt.Sprintf("情绪分布 %d 类，消息 %d 条", len(r.EmotionCounts), r.MessageCount),
		EmotionDistribution: toProtoEmotionDistribution(r.EmotionCounts, totalEmotions),
		EmotionTrend:       nil,
		MessageCount:      int32(r.MessageCount),
		ConversationCount: int32(r.ConversationCount),
		Date:              req.Date,
	}, nil
}

// ReportsTrend 实现 ReportsTrend RPC
func (s *analyticsServer) ReportsTrend(ctx context.Context, req *emotionanalytics.ReportsTrendRequest) (*emotionanalytics.ReportsTrendResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	resp, err := logic.NewReportsTrendLogic(ctx, s.svcCtx).GetTrendReport(&types.GetTrendReportReq{
		UserID:    req.UserId,
		Type:      req.Type,
		StartDate: unixToDateProto(req.DateRange.GetStartDate()),
		EndDate:   unixToDateProto(req.DateRange.GetEndDate()),
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &emotionanalytics.ReportsTrendResponse{Type: req.Type}, nil
		}
		return nil, grpcerr.MapToError(err, "reportsTrend")
	}
	if resp == nil || resp.Report == nil {
		return &emotionanalytics.ReportsTrendResponse{Type: req.Type}, nil
	}
	return &emotionanalytics.ReportsTrendResponse{
		DataPoints: toProtoChartDataPoints(resp.Report.Points),
		Type:       resp.Report.Type,
	}, nil
}

// UserBehaviorDayNight 实现 UserBehaviorDayNight RPC
//
// PR-3.3 阶段：简化为日级聚合（实际 repo 提供 hour-level bucket；proto
// ChartDataPoint timestamp 当作 hour slot）
func (s *analyticsServer) UserBehaviorDayNight(ctx context.Context, req *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorDayNightResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	// PR-3.3: behavior logic 未对接（PR-3.4 阶段补）
	// 当前返空响应 + 注释
	_ = req
	return &emotionanalytics.UserBehaviorDayNightResponse{
		ActiveHours:  nil,
		MessageHours: nil,
	}, nil
}

// UserBehaviorDepth 实现 UserBehaviorDepth RPC
func (s *analyticsServer) UserBehaviorDepth(ctx context.Context, req *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorDepthResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	_ = req
	return &emotionanalytics.UserBehaviorDepthResponse{
		Buckets:       nil,
		AverageLength: 0,
	}, nil
}

// UserBehaviorFrequency 实现 UserBehaviorFrequency RPC
func (s *analyticsServer) UserBehaviorFrequency(ctx context.Context, req *emotionanalytics.UserBehaviorRequest) (*emotionanalytics.UserBehaviorFrequencyResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	_ = req
	return &emotionanalytics.UserBehaviorFrequencyResponse{
		DailyActive: nil,
		StreakDays:  0,
	}, nil
}

// MentalHealthAssessment 实现 MentalHealthAssessment RPC
func (s *analyticsServer) MentalHealthAssessment(ctx context.Context, req *emotionanalytics.MentalHealthAssessmentRequest) (*emotionanalytics.MentalHealthAssessmentResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	assessmentType := "daily"
	if req.Date > 0 {
		assessmentType = "historical"
	}
	resp, err := logic.NewMentalHealthAssessmentLogic(ctx, s.svcCtx).GetLatestAssessment(&types.GetMentalAssessmentReq{
		UserID: req.UserId,
		Type:   assessmentType,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &emotionanalytics.MentalHealthAssessmentResponse{}, nil
		}
		return nil, grpcerr.MapToError(err, "mentalHealthAssessment")
	}
	if resp == nil || resp.Assessment == nil {
		return &emotionanalytics.MentalHealthAssessmentResponse{}, nil
	}
	ass := resp.Assessment
	dimensions := make([]*emotionanalytics.EmotionDistribution, 0, len(ass.Dimensions))
	for _, d := range ass.Dimensions {
		dimensions = append(dimensions, &emotionanalytics.EmotionDistribution{
			Emotion:    d.Name,
			Percentage: d.Score,
		})
	}
	return &emotionanalytics.MentalHealthAssessmentResponse{
		Summary:              ass.Type,
		Score:                ass.OverallScore,
		RiskLevel:            ass.RiskLevel,
		EmotionDistribution: dimensions,
		TriggeredAlertCount:  0,
	}, nil
}

// MentalHealthHistory 实现 MentalHealthHistory RPC（PR-3.3 阶段简化）
func (s *analyticsServer) MentalHealthHistory(ctx context.Context, req *emotionanalytics.MentalHealthHistoryRequest) (*emotionanalytics.MentalHealthHistoryResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	_ = req
	return &emotionanalytics.MentalHealthHistoryResponse{Records: nil}, nil
}

// MentalHealthTrigger 实现 MentalHealthTrigger RPC
func (s *analyticsServer) MentalHealthTrigger(ctx context.Context, req *emotionanalytics.MentalHealthTriggerRequest) (*emotionanalytics.MentalHealthTriggerResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	resp, err := logic.NewMentalHealthTriggerLogic(ctx, s.svcCtx).TriggerAssessment(&types.TriggerMentalHealthReq{
		UserID:         req.UserId,
		AssessmentType: req.TriggerReason,
	})
	if err != nil {
		return nil, grpcerr.MapToError(err, "mentalHealthTrigger")
	}
	if resp == nil {
		return &emotionanalytics.MentalHealthTriggerResponse{Triggered: false, Message: "no-op"}, nil
	}
	return &emotionanalytics.MentalHealthTriggerResponse{
		Triggered: true,
		Message:   resp.Status,
	}, nil
}

// MentalHealthTrend 实现 MentalHealthTrend RPC
func (s *analyticsServer) MentalHealthTrend(ctx context.Context, req *emotionanalytics.MentalHealthTrendRequest) (*emotionanalytics.MentalHealthTrendResponse, error) {
	if s.svcCtx == nil || s.svcCtx.EventRepo == nil {
		return nil, status.Error(codes.Unavailable, "analytics-svc repository not initialized (degraded start)")
	}
	resp, err := logic.NewMentalHealthTrendLogic(ctx, s.svcCtx).GetTrend(&types.GetMentalHealthTrendReq{
		UserID: req.UserId,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &emotionanalytics.MentalHealthTrendResponse{}, nil
		}
		return nil, grpcerr.MapToError(err, "mentalHealthTrend")
	}
	if resp == nil || resp.Report == nil {
		return &emotionanalytics.MentalHealthTrendResponse{}, nil
	}
	t := resp.Report
	return &emotionanalytics.MentalHealthTrendResponse{
		ScoreTrend: toProtoChartDataPoints(t.Points),
		RiskTrend:  nil,
	}, nil
}