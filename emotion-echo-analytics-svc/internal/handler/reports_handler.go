// Package handler — reports_handler.go
//
// Stage 30-A Round 4 GREEN: GET /api/v1/reports/daily
//                              GET /api/v1/reports/trend.
//
// 纯参数解析 + 委派 logic 层；错误转 HTTP 状态码（per stage-30-A §二）。
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"emotion-echo-analytics-svc/internal/logic"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// ReportsDailyHandler GET /api/v1/reports/daily?user_id=&date=
func ReportsDailyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewReportsDailyLogic(c.Request.Context(), svcCtx).GetDailyReport(&types.GetDailyReportReq{
			UserID: userID,
			Date:   c.Query("date"),
		})
		if err != nil {
			status := http.StatusInternalServerError
			if isValidationError(err) {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ReportsTrendHandler GET /api/v1/reports/trend?user_id=&type=&start_date=&end_date=
func ReportsTrendHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewReportsTrendLogic(c.Request.Context(), svcCtx).GetTrendReport(&types.GetTrendReportReq{
			UserID:    userID,
			Type:      c.Query("type"),
			StartDate: c.Query("start_date"),
			EndDate:   c.Query("end_date"),
		})
		if err != nil {
			status := http.StatusInternalServerError
			if isValidationError(err) {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// parseUserID 解析 user_id query 参数（缺失/非法 → 400）
func parseUserID(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("validation: user_id is required")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("validation: user_id must be positive integer")
	}
	return n, nil
}

// isValidationError 区分 logic 层的 validation error 与 upstream error
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// logic 层 validation error 都以 "validation:" 前缀开头
	return len(msg) >= 11 && msg[:11] == "validation:"
}