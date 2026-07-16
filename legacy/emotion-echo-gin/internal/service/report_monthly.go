package service

import (
	"context"
	"time"
)

// GetMonthlyReport 获取月报
func (s *ReportService) GetMonthlyReport(ctx context.Context, userID int64, startDateStr, endDateStr string) (*TrendReport, error) {
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
			days = 30
		}
	} else {
		endDate = time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		startDate = endDate.AddDate(0, 0, -30)
		days = 30
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

	return s.buildTrendReport(ctx, userID, "monthly", startDate, endDate, dates, series)
}
