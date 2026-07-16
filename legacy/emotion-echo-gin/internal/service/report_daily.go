package service

import (
	"context"
	"fmt"
	"time"
)

// GetDailyReport 获取日报
func (s *ReportService) GetDailyReport(ctx context.Context, userID int64, date string) (*DailyReport, error) {
	reportDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format")
	}

	startOfDay := reportDate.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	convCount, err := s.convRepo.CountByUserIDAndDate(ctx, userID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}

	msgCount, wordCount, err := s.msgRepo.CountByUserIDAndDate(ctx, userID, startOfDay.UnixMilli(), endOfDay.UnixMilli())
	if err != nil {
		return nil, err
	}

	emotionDist, err := s.getEmotionDistribution(ctx, userID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}

	intentCounts, err := s.msgRepo.CountIntentTypeByUserIDAndDate(ctx, userID, startOfDay.UnixMilli(), endOfDay.UnixMilli())
	if err != nil {
		return nil, err
	}

	intentDistribution := CalculateIntentDistribution(intentCounts)
	summary := GenerateDailySummary(emotionDist, convCount, msgCount)

	return &DailyReport{
		Date:                date,
		Summary:             summary,
		EmotionDistribution: emotionDist,
		IntentDistribution:  intentDistribution,
		ConversationCount:   convCount,
		MessageCount:        msgCount,
		WordCount:           wordCount,
	}, nil
}
