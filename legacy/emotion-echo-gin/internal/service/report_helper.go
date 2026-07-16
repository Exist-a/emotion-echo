package service

import (
	"context"
	"time"
)

// buildTrendReport 构建趋势报表的公共逻辑
func (s *ReportService) buildTrendReport(ctx context.Context, userID int64, reportType string, startDate, endDate time.Time, dates []string, series []SeriesData) (*TrendReport, error) {
	convCount, err := s.convRepo.CountByUserIDAndDate(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	msgCount, wordCount, err := s.msgRepo.CountByUserIDAndDate(ctx, userID, startDate.UnixMilli(), endDate.UnixMilli())
	if err != nil {
		return nil, err
	}

	emotionDist, err := s.getEmotionDistribution(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	intentCounts, err := s.msgRepo.CountIntentTypeByUserIDAndDate(ctx, userID, startDate.UnixMilli(), endDate.UnixMilli())
	if err != nil {
		return nil, err
	}

	intentDistribution := CalculateIntentDistribution(intentCounts)
	summary := GenerateTrendSummary(reportType, emotionDist, convCount, msgCount)

	return &TrendReport{
		Type:                reportType,
		Dates:               dates,
		Series:              series,
		Summary:             summary,
		EmotionDistribution: emotionDist,
		IntentDistribution:  intentDistribution,
		ConversationCount:   convCount,
		MessageCount:        msgCount,
		WordCount:           wordCount,
	}, nil
}

// getEmotionDistribution 获取情绪分布
func (s *ReportService) getEmotionDistribution(ctx context.Context, userID int64, startDate, endDate time.Time) ([]EmotionDistribution, error) {
	analyses, err := s.analysisRepo.ListByUserIDAndDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	emotionCount := make(map[string]int)
	total := 0

	for _, a := range analyses {
		if a.DominantEmotion != "" {
			emotionCount[a.DominantEmotion]++
			total++
		}
	}

	if total == 0 {
		return []EmotionDistribution{
			{Name: "中性", Value: 100},
		}, nil
	}

	dist := make([]EmotionDistribution, 0)
	for emotion, count := range emotionCount {
		percentage := int(float64(count) / float64(total) * 100)
		dist = append(dist, EmotionDistribution{
			Name:  emotion,
			Value: percentage,
		})
	}

	return dist, nil
}
