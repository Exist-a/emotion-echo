// Package handler — userbehavior_handler.go
//
// Stage 30-A Round 4 GREEN: GET /api/v1/user-behavior/{day-night,depth,frequency}
package handler

import (
	"net/http"

	"emotion-echo-analytics-svc/internal/logic"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// UserBehaviorDayNightHandler GET /api/v1/user-behavior/day-night
func UserBehaviorDayNightHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewUserBehaviorDayNightLogic(c.Request.Context(), svcCtx).GetDayNightPattern(&types.GetDayNightPatternReq{
			UserID:    userID,
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

// UserBehaviorDepthHandler GET /api/v1/user-behavior/depth
func UserBehaviorDepthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewUserBehaviorDepthLogic(c.Request.Context(), svcCtx).GetInteractionDepth(&types.GetInteractionDepthReq{
			UserID:    userID,
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

// UserBehaviorFrequencyHandler GET /api/v1/user-behavior/frequency
func UserBehaviorFrequencyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := parseUserID(c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewUserBehaviorFrequencyLogic(c.Request.Context(), svcCtx).GetFrequencyTrend(&types.GetFrequencyTrendReq{
			UserID:    userID,
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