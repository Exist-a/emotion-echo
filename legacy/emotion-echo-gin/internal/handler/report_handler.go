package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// ReportHandler 报表处理器
type ReportHandler struct {
	reportService *service.ReportService
}

// NewReportHandler 创建报表处理器
func NewReportHandler(reportService *service.ReportService) *ReportHandler {
	return &ReportHandler{reportService: reportService}
}

// GetDaily 获取日报
func (h *ReportHandler) GetDaily(c *gin.Context) {
	userID := c.GetInt64("userId")

	date := c.Query("date")
	if date == "" {
		// 默认今天
		date = time.Now().Format("2006-01-02")
	}

	report, err := h.reportService.GetDailyReport(c.Request.Context(), userID, date)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, report)
}

// GetTrend 获取趋势报表
func (h *ReportHandler) GetTrend(c *gin.Context) {
	userID := c.GetInt64("userId")

	reportType := c.Query("type")
	if reportType == "" {
		reportType = "weekly"
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	report, err := h.reportService.GetTrendReport(c.Request.Context(), userID, reportType, startDate, endDate)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, report)
}
