package service

import (
	"context"
	"fmt"

	"emotion-echo-gin/internal/repository"
)

// ReportService 报表服务
type ReportService struct {
	convRepo      *repository.ConversationRepository
	msgRepo       *repository.MessageRepository
	analysisRepo  *repository.EmotionAnalysisRepository
}

// NewReportService 创建报表服务
func NewReportService(convRepo *repository.ConversationRepository, msgRepo *repository.MessageRepository, analysisRepo *repository.EmotionAnalysisRepository) *ReportService {
	return &ReportService{
		convRepo:     convRepo,
		msgRepo:      msgRepo,
		analysisRepo: analysisRepo,
	}
}

// GetTrendReport 获取趋势报表（统一入口，内部路由到周/月/年报）
func (s *ReportService) GetTrendReport(ctx context.Context, userID int64, reportType, startDateStr, endDateStr string) (*TrendReport, error) {
	switch reportType {
	case "weekly":
		return s.GetWeeklyReport(ctx, userID, startDateStr, endDateStr)
	case "monthly":
		return s.GetMonthlyReport(ctx, userID, startDateStr, endDateStr)
	case "yearly":
		return s.GetYearlyReport(ctx, userID, startDateStr, endDateStr)
	default:
		return nil, fmt.Errorf("invalid report type: %s", reportType)
	}
}
