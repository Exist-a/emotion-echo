package service

import (
	"context"
	"time"
)

// GetWeeklyReport 获取周报
func (s *ReportService) GetWeeklyReport(ctx context.Context, userID int64, startDateStr, endDateStr string) (*TrendReport, error) {
	var startDate, endDate time.Time
	var days int
	var err error

	if startDateStr != "" && endDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return nil, err
		}
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return nil, err
		}
		endDate = endDate.Add(24 * time.Hour)
		days = int(endDate.Sub(startDate).Hours() / 24)
		if days <= 0 {
			days = 7
		}
	} else {
		endDate = time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		startDate = endDate.AddDate(0, 0, -7)
		days = 7
	}

	dates := make([]string, days)
	for i := 0; i < days; i++ {
		d := startDate.AddDate(0, 0, i)
		dates[i] = d.Format("01-02")
	}

	analyses, err := s.analysisRepo.ListByUserIDAndDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	series := AggregateEmotionTrendByDay(analyses, days, startDate)

	return s.buildTrendReport(ctx, userID, "weekly", startDate, endDate, dates, series)
}
