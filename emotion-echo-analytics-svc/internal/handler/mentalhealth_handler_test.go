// Package handler — mentalhealth_handler_test.go
//
// Sibling test for mentalhealth_handler.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 4 RED: cover 4 mental-health routes:
//   - GET  /api/v1/mental-health/assessment?user_id=&type=
//   - GET  /api/v1/mental-health/history?user_id=&type=&cursor=&limit=
//   - POST /api/v1/mental-health/trigger
//   - GET  /api/v1/mental-health/trend?user_id=&type=&start_date=&end_date=
package handler

import (
	"bytes"
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
	"emotion-echo-analytics-svc/internal/trigger"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMHHandlerCtx(repo repository.MentalHealthRepo, queue *trigger.TriggerQueue) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:           config.Config{},
		MentalHealthRepo: repo,
		TriggerQueue:     queue,
	}
}

// mhStubRepo2 mentalhealth handler 专用 stub（独立类型）。
type mhStubRepo2 struct {
	assessment    *repository.MentalAssessment
	assessmentErr error

	history         []repository.AssessmentHistoryItem
	historyNextCur  string
	historyErr      error

	trend    []repository.TrendPoint
	trendErr error
}

func (s *mhStubRepo2) GetLatestAssessment(_ context.Context, _ int64, _ repository.AssessmentType) (*repository.MentalAssessment, error) {
	return s.assessment, s.assessmentErr
}

func (s *mhStubRepo2) ListAssessmentHistory(_ context.Context, _ int64, _, _ string, _ int) ([]repository.AssessmentHistoryItem, string, error) {
	return s.history, s.historyNextCur, s.historyErr
}

func (s *mhStubRepo2) GetTrendData(_ context.Context, _ int64, _ string, _, _ time.Time) ([]repository.TrendPoint, error) {
	return s.trend, s.trendErr
}

func (s *mhStubRepo2) Ping(_ context.Context) error { return nil }

// =====================================================
// GET /api/v1/mental-health/assessment
// =====================================================

func TestMentalHealthAssessmentHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{assessment: &repository.MentalAssessment{
		UserID: 1, Type: "daily",
		WindowStart: "2026-07-15", WindowEnd: "2026-07-15",
		OverallScore: 35.5, RiskLevel: "moderate",
		Dimensions:    []repository.DimensionScore{},
	}}
	r := gin.New()
	r.GET("/api/v1/mental-health/assessment", MentalHealthAssessmentHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/assessment?user_id=1&type=daily", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Contains(t, got, "assessment")
}

func TestMentalHealthAssessmentHandler_InvalidType_Returns400(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{}
	r := gin.New()
	r.GET("/api/v1/mental-health/assessment", MentalHealthAssessmentHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/assessment?user_id=1&type=garbage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMentalHealthAssessmentHandler_NoData_Returns200WithNil(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{assessment: nil}
	r := gin.New()
	r.GET("/api/v1/mental-health/assessment", MentalHealthAssessmentHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/assessment?user_id=1&type=daily", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	// assessment 字段应该是 nil（不是 {} 也不是 missing）
	_, exists := got["assessment"]
	assert.True(t, exists, "JSON 必须包含 assessment 字段（即使值是 null）")
}

// =====================================================
// GET /api/v1/mental-health/history
// =====================================================

func TestMentalHealthHistoryHandler_HappyPath_Returns200WithCursor(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{
		history:        []repository.AssessmentHistoryItem{{ID: 1, UserID: 1, AssessmentType: "PHQ-9", OverallScore: 20}},
		historyNextCur: "next-xyz",
	}
	r := gin.New()
	r.GET("/api/v1/mental-health/history", MentalHealthHistoryHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/history?user_id=1&type=PHQ-9&limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "next-xyz", got["nextCursor"])
}

// =====================================================
// POST /api/v1/mental-health/trigger
// =====================================================

func TestMentalHealthTriggerHandler_HappyPath_Returns202(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 0, 4, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	r := gin.New()
	r.POST("/api/v1/mental-health/trigger", MentalHealthTriggerHandler(newMHHandlerCtx(nil, queue)))

	body := `{"userId":42,"assessmentType":"daily"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mental-health/trigger", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code, "POST trigger 返回 202 Accepted per stage-30-A §二")
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "accepted", got["status"])
	assert.NotEmpty(t, got["taskId"])
}

func TestMentalHealthTriggerHandler_QueueFull_Returns503(t *testing.T) {
	t.Parallel()
	// workers=0 + cap=1 → 第一个 Submit 入队，第二个 ErrQueueFull
	queue := trigger.NewTriggerQueue(context.Background(), 0, 1, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	r := gin.New()
	r.POST("/api/v1/mental-health/trigger", MentalHealthTriggerHandler(newMHHandlerCtx(nil, queue)))

	body1 := `{"userId":1,"assessmentType":"daily"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/mental-health/trigger", bytes.NewReader([]byte(body1)))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusAccepted, w1.Code)

	body2 := `{"userId":2,"assessmentType":"daily"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/mental-health/trigger", bytes.NewReader([]byte(body2)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusServiceUnavailable, w2.Code, "backpressure 返回 503")
}

func TestMentalHealthTriggerHandler_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	r := gin.New()
	r.POST("/api/v1/mental-health/trigger", MentalHealthTriggerHandler(newMHHandlerCtx(nil, queue)))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mental-health/trigger", bytes.NewReader([]byte("{garbage")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// =====================================================
// GET /api/v1/mental-health/trend
// =====================================================

func TestMentalHealthTrendHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{trend: []repository.TrendPoint{
		{Date: "2026-07-01", PrimaryEmotion: "happy", AvgSentiment: 0.5, AvgConfidence: 0.9, Count: 5},
	}}
	r := gin.New()
	r.GET("/api/v1/mental-health/trend", MentalHealthTrendHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/trend?user_id=1&type=weekly&start_date=2026-07-01&end_date=2026-07-07", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMentalHealthTrendHandler_InvalidType_Returns400(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{}
	r := gin.New()
	r.GET("/api/v1/mental-health/trend", MentalHealthTrendHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/trend?user_id=1&type=daily&start_date=2026-07-01&end_date=2026-07-07", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMentalHealthTrendHandler_RepoError_Returns500(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo2{trendErr: errors.New("postgres unavailable")}
	r := gin.New()
	r.GET("/api/v1/mental-health/trend", MentalHealthTrendHandler(newMHHandlerCtx(repo, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/trend?user_id=1&type=weekly&start_date=2026-07-01&end_date=2026-07-07", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// 引用 types
var _ = types.TriggerMentalHealthReq{}