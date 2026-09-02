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
	"time"

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
		Fail(c, http.StatusBadRequest, 1, "validation: user_id is required")
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
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	// fix/chart-contract-alignment: 直接吐前端契约形状（不用单数 key 包一层）
	OK(c, toFrontendDailyReport(report))
}

func (h *AnalyticsHandler) trendReport(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	// fix/chart-contract-alignment: 前端传 month/year/weekly alias，BFF 内部转 start_date/end_date
	reportType, startDate, endDate := normalizeTrendQuery(c)
	report, err := h.analytics.TrendReport(session.WithRequestAuth(c), uid,
		reportType, startDate, endDate)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, toFrontendTrendReport(report))
}

func (h *AnalyticsHandler) dayNight(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	pattern, err := h.analytics.DayNightPattern(session.WithRequestAuth(c), uid,
		c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"pattern": pattern})
}

func (h *AnalyticsHandler) interactionDepth(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	depth, err := h.analytics.InteractionDepth(session.WithRequestAuth(c), uid,
		c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"depth": depth})
}

func (h *AnalyticsHandler) frequencyTrend(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	counts, err := h.analytics.FrequencyTrend(session.WithRequestAuth(c), uid,
		c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"counts": counts})
}

func (h *AnalyticsHandler) mentalAssessment(c *gin.Context) {
	uid, ok := userIDQuery(c)
	if !ok {
		return
	}
	assessment, err := h.analytics.MentalAssessment(session.WithRequestAuth(c), uid, c.Query("type"))
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"assessment": assessment})
}

// fix/chart-contract-alignment: trendReport alias 解析。
//
// 前端 4 个 dashboard 页面调 /reports/trend 时 query 参数不一致：
//   - weeklyReport:    type=weekly + start=YYYY-MM-DD + end=YYYY-MM-DD
//   - monthlyReport:   type=monthly + month=YYYY-MM
//   - annualReport:    type=yearly + year=YYYY
//
// analytics-svc 期望的是 type=weekly|monthly|yearly + start_date + end_date。
// 本函数把前端参数归一化为 (type, start_date, end_date)：
//   - start / end → start_date / end_date（直接转发）
//   - month=YYYY-MM → start_date=YYYY-MM-01, end_date=YYYY-MM-{last day}
//   - year=YYYY → start_date=YYYY-01-01, end_date=YYYY-12-31
//
// 不识别 → 原样回传（让 analytics-svc 自己 4xx）。
func normalizeTrendQuery(c *gin.Context) (reportType, startDate, endDate string) {
	reportType = c.Query("type")
	startDate = c.Query("start_date")
	endDate = c.Query("end_date")
	if startDate == "" {
		if s := c.Query("start"); s != "" {
			startDate = s
		}
	}
	if endDate == "" {
		if e := c.Query("end"); e != "" {
			endDate = e
		}
	}
	if month := c.Query("month"); month != "" && startDate == "" {
		startDate = month + "-01"
		endDate = monthEndDay(month)
	}
	if year := c.Query("year"); year != "" && startDate == "" {
		startDate = year + "-01-01"
		endDate = year + "-12-31"
	}
	return reportType, startDate, endDate
}

// monthEndDay 给定 "YYYY-MM" 返回该月最后一天的 "YYYY-MM-DD"
//
// 用 time.Parse("2006-01", month) → 加一个月减一天。
// 解析失败返回 "2006-01-31"（保守 fallback，不会引起 silent 错误数据）。
func monthEndDay(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "2006-01-31"
	}
	// 下月 1 号减 1 天
	nextMonth := t.AddDate(0, 1, 0)
	lastDay := nextMonth.AddDate(0, 0, -1)
	return lastDay.Format("2006-01-02")
}
