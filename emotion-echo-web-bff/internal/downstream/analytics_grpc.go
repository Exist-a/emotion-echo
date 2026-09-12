// Package downstream — analytics_grpc.go
//
// Stage 62 PR-3.3: BFF → analytics-svc gRPC client（6 RPC 走 gRPC）
//
// proto emotionanalytics.AnalyticsService 暴露 9 RPC；BFF AnalyticsClient interface
// 只暴露 6 个常用方法（PR-3.3 阶段覆盖 6 个；其余 3 个 MentalHealthHistory/Trend/Trigger
// 等前端触发新需求时再补 client 方法）。

package downstream

import (
	"context"
	"time"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"

	"google.golang.org/grpc"
)

// analyticsGRPCClient 是 AnalyticsClient 的 gRPC 实现
type analyticsGRPCClient struct {
	conn *grpc.ClientConn
}

// NewAnalyticsGRPCClient 构造（conn=nil → 返 nil）
func NewAnalyticsGRPCClient(conn *grpc.ClientConn) AnalyticsClient {
	if conn == nil {
		return nil
	}
	return &analyticsGRPCClient{conn: conn}
}

// parseDate YYYY-MM-DD → unix seconds（start of day UTC）
func parseDate(s string) int64 {
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
func unixToDate(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

// DailyReport gRPC（proto ReportsDaily）
func (c *analyticsGRPCClient) DailyReport(ctx context.Context, userID int64, date string) (*DailyReport, error) {
	cli := emotionanalytics.NewAnalyticsServiceClient(c.conn)
	resp, err := cli.ReportsDaily(withUserID(ctx), &emotionanalytics.ReportsDailyRequest{
		UserId: userID,
		Date:   parseDate(date),
	})
	if err != nil {
		return nil, wrapGRPCError(err, "analytics dailyReport")
	}
	if resp == nil {
		return nil, nil
	}
	emotions := make(map[string]int64, len(resp.EmotionDistribution))
	for _, d := range resp.EmotionDistribution {
		emotions[d.Emotion] = int64(d.Count)
	}
	intents := make(map[string]int64, len(resp.IntentDistribution))
	for _, d := range resp.IntentDistribution {
		intents[d.Intent] = int64(d.Count)
	}
	return &DailyReport{
		UserID:            userID,
		Date:              date,
		EmotionCounts:     emotions,
		MessageCount:      int64(resp.MessageCount),
		ConversationCount: int64(resp.ConversationCount),
		IntentCounts:      intents, // Stage 82 PR-3b
	}, nil
}

// TrendReport gRPC（proto ReportsTrend）
func (c *analyticsGRPCClient) TrendReport(ctx context.Context, userID int64, reportType, startDate, endDate string) (*TrendReport, error) {
	cli := emotionanalytics.NewAnalyticsServiceClient(c.conn)
	resp, err := cli.ReportsTrend(withUserID(ctx), &emotionanalytics.ReportsTrendRequest{
		UserId: userID,
		Type:   reportType,
		DateRange: &emotionanalytics.DateRange{
			StartDate: parseDate(startDate),
			EndDate:   parseDate(endDate),
		},
	})
	if err != nil {
		return nil, wrapGRPCError(err, "analytics trendReport")
	}
	if resp == nil {
		return nil, nil
	}
	points := make([]TrendPoint, 0, len(resp.DataPoints))
	for _, p := range resp.DataPoints {
		points = append(points, TrendPoint{
			Date:           unixToDate(p.Timestamp),
			AvgSentiment:   p.Value,
			AvgConfidence:  p.Value,
			Count:          int64(p.Value),
			PrimaryEmotion: p.Label,
		})
	}
	return &TrendReport{
		UserID:    userID,
		Type:      resp.Type,
		StartDate: startDate,
		EndDate:   endDate,
		Points:    points,
	}, nil
}

// DayNightPattern gRPC（返回 24 桶）
func (c *analyticsGRPCClient) DayNightPattern(ctx context.Context, userID int64, startDate, endDate string) (map[int]int64, error) {
	cli := emotionanalytics.NewAnalyticsServiceClient(c.conn)
	resp, err := cli.UserBehaviorDayNight(withUserID(ctx), &emotionanalytics.UserBehaviorRequest{
		UserId: userID,
		DateRange: &emotionanalytics.DateRange{
			StartDate: parseDate(startDate),
			EndDate:   parseDate(endDate),
		},
	})
	if err != nil {
		return nil, wrapGRPCError(err, "analytics dayNightPattern")
	}
	out := make(map[int]int64, 24)
	for _, p := range resp.ActiveHours {
		hour := int(p.Timestamp) % 24
		out[hour] = int64(p.Value)
	}
	return out, nil
}

// InteractionDepth gRPC
func (c *analyticsGRPCClient) InteractionDepth(ctx context.Context, userID int64, startDate, endDate string) (*InteractionDepth, error) {
	cli := emotionanalytics.NewAnalyticsServiceClient(c.conn)
	resp, err := cli.UserBehaviorDepth(ctx, &emotionanalytics.UserBehaviorRequest{
		UserId: userID,
		DateRange: &emotionanalytics.DateRange{
			StartDate: parseDate(startDate),
			EndDate:   parseDate(endDate),
		},
	})
	if err != nil {
		return nil, wrapGRPCError(err, "analytics interactionDepth")
	}
	if resp == nil {
		return nil, nil
	}
	// InteractionDepth 已有 TotalMessages/TotalConversations/AvgMessagesPerConv 字段
	// proto UserBehaviorDepthResponse 暂未对齐 → 留 PR-3.4 阶段补
	return &InteractionDepth{
		AvgMessagesPerConv: resp.AverageLength,
	}, nil
}

// FrequencyTrend gRPC
func (c *analyticsGRPCClient) FrequencyTrend(ctx context.Context, userID int64, startDate, endDate string) ([]DailyCount, error) {
	cli := emotionanalytics.NewAnalyticsServiceClient(c.conn)
	resp, err := cli.UserBehaviorFrequency(ctx, &emotionanalytics.UserBehaviorRequest{
		UserId: userID,
		DateRange: &emotionanalytics.DateRange{
			StartDate: parseDate(startDate),
			EndDate:   parseDate(endDate),
		},
	})
	if err != nil {
		return nil, wrapGRPCError(err, "analytics frequencyTrend")
	}
	out := make([]DailyCount, 0, len(resp.DailyActive))
	for _, p := range resp.DailyActive {
		out = append(out, DailyCount{
			Date:  unixToDate(p.Timestamp),
			Count: int64(p.Value),
		})
	}
	return out, nil
}

// MentalAssessment gRPC
func (c *analyticsGRPCClient) MentalAssessment(ctx context.Context, userID int64, assessmentType string) (*MentalAssessment, error) {
	cli := emotionanalytics.NewAnalyticsServiceClient(c.conn)
	resp, err := cli.MentalHealthAssessment(ctx, &emotionanalytics.MentalHealthAssessmentRequest{
		UserId: userID,
		Date:   0,
	})
	if err != nil {
		return nil, wrapGRPCError(err, "analytics mentalAssessment")
	}
	if resp == nil {
		return nil, nil
	}
	dimensions := make([]DimensionScore, 0, len(resp.EmotionDistribution))
	for _, d := range resp.EmotionDistribution {
		dimensions = append(dimensions, DimensionScore{
			Name:  d.Emotion,
			Score: d.Percentage,
		})
	}
	return &MentalAssessment{
		UserID:       userID,
		Type:         assessmentType,
		OverallScore: resp.Score,
		RiskLevel:    resp.RiskLevel,
		Dimensions:   dimensions,
	}, nil
}