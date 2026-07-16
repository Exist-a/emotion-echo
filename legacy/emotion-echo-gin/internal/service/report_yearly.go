package service

import (
	"context"
	"time"
)

// GetYearlyReport 获取年报
func (s *ReportService) GetYearlyReport(ctx context.Context, userID int64, startDateStr, endDateStr string) (*TrendReport, error) {
	var startDate, endDate time.Time
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
	} else {
		endDate = time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
		startDate = endDate.AddDate(0, -12, 0)
	}

	months := 12
	dates := make([]string, months)
	for i := 0; i < months; i++ {
		d := startDate.AddDate(0, i, 0)
		dates[i] = d.Format("2006-01")
	}

	analyses, err := s.analysisRepo.ListByUserIDAndDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	series := AggregateEmotionTrendByMonth(analyses, months, startDate)

	return s.buildTrendReport(ctx, userID, "yearly", startDate, endDate, dates, series)
}
