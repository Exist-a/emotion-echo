// Package handler — reports_handler_test.go
//
// Sibling test for reports_handler.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 4 RED: cover the 2 reports handler routes:
//   - GET /api/v1/reports/daily?date=YYYY-MM-DD
//   - GET /api/v1/reports/trend?type=weekly|monthly|yearly&start_date=&end_date=
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newReportsHandlerCtx(repo repository.ReportRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, ReportRepo: repo}
}

// fakeReportRepo2 是 reports handler 专用 stub（独立类型，避开
// logic 包里的同类型 — 测试 binary 编译时每个 _test.go 独立）。
type fakeReportRepo2 struct {
	daily    *repository.DailyReport
	dailyErr error

	trend    *repository.TrendReport
	trendErr error
}

func (f *fakeReportRepo2) GetDailyReport(_ context.Context, _ int64, _ time.Time) (*repository.DailyReport, error) {
	return f.daily, f.dailyErr
}

func (f *fakeReportRepo2) GetTrendReport(_ context.Context, _ int64, _ string, _, _ time.Time) (*repository.TrendReport, error) {
	return f.trend, f.trendErr
}

func (f *fakeReportRepo2) Ping(_ context.Context) error { return nil }

// =====================================================
// GET /api/v1/reports/daily
// =====================================================

func TestReportsDailyHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo2{
		daily: &repository.DailyReport{
			UserID: 1, Date: "2026-07-15",
			EmotionCounts: map[string]int64{"happy": 3},
			MessageCount: 10, ConversationCount: 2, AssessmentCount: 1,
			AvgSentiment: 0.42, AvgConfidence: 0.85,
		},
	}
	r := gin.New()
	r.GET("/api/v1/reports/daily", ReportsDailyHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-07-15&user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, jsonDecode(w.Body.Bytes(), &got))
	assert.Contains(t, got, "report")
}

func TestReportsDailyHandler_MissingDate_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	// 没 date → logic 默认 today; repo 必须仍被调一次
	repo := &fakeReportRepo2{
		daily: &repository.DailyReport{UserID: 1, Date: "today", EmotionCounts: map[string]int64{}},
	}
	r := gin.New()
	r.GET("/api/v1/reports/daily", ReportsDailyHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestReportsDailyHandler_BadDateFormat_Returns400(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo2{}
	r := gin.New()
	r.GET("/api/v1/reports/daily", ReportsDailyHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=garbage&user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReportsDailyHandler_RepoError_Returns500(t *testing.T) {
	t.Parallel()
	boom := errors.New("kafka consumer lag")
	repo := &fakeReportRepo2{dailyErr: boom}
	r := gin.New()
	r.GET("/api/v1/reports/daily", ReportsDailyHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-07-15&user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// =====================================================
// GET /api/v1/reports/trend
// =====================================================

func TestReportsTrendHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo2{
		trend: &repository.TrendReport{
			UserID: 1, Type: "weekly",
			StartDate: "2026-07-01", EndDate: "2026-07-14",
			Points: []repository.TrendPoint{
				{Date: "2026-07-01", PrimaryEmotion: "happy", AvgSentiment: 0.5, AvgConfidence: 0.9, Count: 5},
			},
		},
	}
	r := gin.New()
	r.GET("/api/v1/reports/trend", ReportsTrendHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/trend?type=weekly&start_date=2026-07-01&end_date=2026-07-14&user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestReportsTrendHandler_InvalidType_Returns400(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo2{}
	r := gin.New()
	r.GET("/api/v1/reports/trend", ReportsTrendHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/trend?type=bogus&start_date=2026-07-01&end_date=2026-07-14&user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReportsTrendHandler_InvertedRange_Returns400(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo2{}
	r := gin.New()
	r.GET("/api/v1/reports/trend", ReportsTrendHandler(newReportsHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/trend?type=weekly&start_date=2026-07-14&end_date=2026-07-01&user_id=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// 引用 types 以满足 unused check
var _ = types.GetDailyReportReq{}