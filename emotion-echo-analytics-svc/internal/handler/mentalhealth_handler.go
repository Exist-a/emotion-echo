// Package handler — mentalhealth_handler.go
//
// Stage 30-A Round 4 GREEN:
//   - GET  /api/v1/mental-health/assessment
//   - GET  /api/v1/mental-health/history
//   - POST /api/v1/mental-health/trigger
//   - GET  /api/v1/mental-health/trend
//
// 状态码策略（per stage-30-A §二 + §三.2）：
//   - 200 OK：正常返回（含 assessment=nil 当用户无评估）
//   - 202 Accepted：POST trigger async 返回
//   - 400 Bad Request：validation error（logic 层前缀 "validation:"）
//   - 500 Internal Server Error：upstream / repo error
//   - 503 Service Unavailable：TriggerQueue backpressure / closed
package handler

import (
	"errors"
	"net/http"

	"emotion-echo-analytics-svc/internal/logic"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/trigger"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// MentalHealthAssessmentHandler GET /api/v1/mental-health/assessment
func MentalHealthAssessmentHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewMentalHealthAssessmentLogic(c.Request.Context(), svcCtx).GetLatestAssessment(&types.GetMentalAssessmentReq{
			UserID: userID,
			Type:   c.Query("type"),
		})
		if err != nil {
			status := http.StatusInternalServerError
			if isValidationError(err) {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		// resp.Assessment 可能为 nil（用户无评估）— handler 仍返 200
		c.JSON(http.StatusOK, resp)
	}
}

// MentalHealthHistoryHandler GET /api/v1/mental-health/history
func MentalHealthHistoryHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		limit := 0 // 0 → logic 内 clamp 到 20
		if v := c.Query("limit"); v != "" {
			// parseLimit 让 logic 内部 clamp 范围生效；这里只 parse 失败返 400
			if _, perr := parsePositiveInt(v); perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": perr.Error()})
				return
			}
		}
		resp, err := logic.NewMentalHealthHistoryLogic(c.Request.Context(), svcCtx).ListHistory(&types.GetMentalHealthHistoryReq{
			UserID: userID,
			Type:   c.Query("type"),
			Cursor: c.Query("cursor"),
			Limit:  limit,
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

// MentalHealthTriggerHandler POST /api/v1/mental-health/trigger
//
// Async（per stage-30-A §三.2）：立即返回 202 + task_id。
// TriggerQueue backpressure（ErrQueueFull）或已关闭（ErrQueueClosed）
// → 503 Service Unavailable（caller 应 retry-with-backoff）。
func MentalHealthTriggerHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.TriggerMentalHealthReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewMentalHealthTriggerLogic(c.Request.Context(), svcCtx).TriggerAssessment(&req)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, trigger.ErrQueueFull):
				status = http.StatusServiceUnavailable
			case errors.Is(err, trigger.ErrQueueClosed):
				status = http.StatusServiceUnavailable
			case isValidationError(err):
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, resp)
	}
}

// MentalHealthTrendHandler GET /api/v1/mental-health/trend
func MentalHealthTrendHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewMentalHealthTrendLogic(c.Request.Context(), svcCtx).GetTrend(&types.GetMentalHealthTrendReq{
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

// parsePositiveInt 用于 query 参数解析失败检测（logic 内仍做 clamp）
func parsePositiveInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("validation: limit must be positive integer")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}