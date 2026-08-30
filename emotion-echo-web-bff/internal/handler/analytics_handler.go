// Package handler — analytics_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.47-51: analytics handler（BFF → analytics-svc）
//
// 端点：
//   GET /api/v1/reports/daily?user_id=&date=
//   GET /api/v1/reports/trend?user_id=&type=&start_date=&end_date=
//   GET /api/v1/user-behavior/day-night?user_id=&start_date=&end_date=
//   GET /api/v1/user-behavior/depth?user_id=&start_date=&end_date=
//   GET /api/v1/user-behavior/frequency?user_id=&start_date=&end_date=
//   GET /api/v1/mental-health/assessment?user_id=&type=
//
// query 参数名与下游一致（snake_case）。
package handler

import (
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// AnalyticsHandler 处理 /api/v1/reports/* 与 /api/v1/user-behavior/* 端点
type AnalyticsHandler struct {
	analytics downstream.AnalyticsClient
}

// NewAnalyticsHandler 构造
func NewAnalyticsHandler(analytics downstream.AnalyticsClient) *AnalyticsHandler {
	return &AnalyticsHandler{analytics: analytics}
}

// Register 注册路由
func (h *AnalyticsHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/reports/daily", h.dailyReport)
	r.GET("/api/v1/reports/trend", h.trendReport)
	r.GET("/api/v1/user-behavior/day-night", h.dayNight)
	r.GET("/api/v1/user-behavior/depth", h.interactionDepth)
	r.GET("/api/v1/user-behavior/frequency", h.frequencyTrend)
	r.GET("/api/v1/mental-health/assessment", h.mentalAssessment)
}

// userIDQuery 解析必填 user_id；缺失/非法时写 400 并返回 false
func userIDQuery(c *gin.Context) (int64, bool) {
	v := c.Query("user_id")
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: user_id is required"})
		return 0, false
	}
	return id, true
}

func (h *AnalyticsHandler) dailyReport(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	report, err := h.analytics.DailyReport(session.WithRequestAuth(c), uid, c.Query("date"))
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"report": report})
}

func (h *AnalyticsHandler) trendReport(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	report, err := h.analytics.TrendReport(session.WithRequestAuth(c), uid,
		c.Query("type"), c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"report": report})
}

func (h *AnalyticsHandler) dayNight(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	pattern, err := h.analytics.DayNightPattern(session.WithRequestAuth(c), uid,
		c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pattern": pattern})
}

func (h *AnalyticsHandler) interactionDepth(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	depth, err := h.analytics.InteractionDepth(session.WithRequestAuth(c), uid,
		c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"depth": depth})
}

func (h *AnalyticsHandler) frequencyTrend(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	counts, err := h.analytics.FrequencyTrend(session.WithRequestAuth(c), uid,
		c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"counts": counts})
}

func (h *AnalyticsHandler) mentalAssessment(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	assessment, err := h.analytics.MentalAssessment(session.WithRequestAuth(c), uid, c.Query("type"))
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"assessment": assessment})
}
