// Package handler — userbehavior_handler_test.go
//
// Sibling test for userbehavior_handler.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 4 RED: cover 3 user-behavior routes:
//   - GET /api/v1/user-behavior/day-night?user_id=&start_date=&end_date=
//   - GET /api/v1/user-behavior/depth?user_id=&start_date=&end_date=
//   - GET /api/v1/user-behavior/frequency?user_id=&start_date=&end_date=
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
	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUBHandlerCtx(repo repository.EventRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, EventRepo: repo}
}

// ubStubEventRepo 是 userbehavior handler 专用 stub（独立类型）。
type ubStubEventRepo struct {
	dayNight    map[int]int64
	dayNightErr error

	depth    *repository.InteractionDepth
	depthErr error

	freq    []repository.DailyCount
	freqErr error
}

func (s *ubStubEventRepo) GetDayNightPattern(_ context.Context, _ int64, _, _ time.Time) (map[int]int64, error) {
	return s.dayNight, s.dayNightErr
}

func (s *ubStubEventRepo) GetInteractionDepth(_ context.Context, _ int64, _, _ time.Time) (*repository.InteractionDepth, error) {
	return s.depth, s.depthErr
}

func (s *ubStubEventRepo) GetFrequencyTrend(_ context.Context, _ int64, _, _ time.Time) ([]repository.DailyCount, error) {
	return s.freq, s.freqErr
}

// EventRepo 其他方法 — 测试不调用
func (s *ubStubEventRepo) GetByID(_ context.Context, _ int64) (*model.UserBehaviorEvent, error) {
	return nil, nil
}

func (s *ubStubEventRepo) Create(_ context.Context, _ *model.UserBehaviorEvent) error { return nil }

func (s *ubStubEventRepo) Ping(_ context.Context) error { return nil }

// =====================================================
// GET /api/v1/user-behavior/day-night
// =====================================================

func TestUserBehaviorDayNightHandler_HappyPath_Returns200_All24Buckets(t *testing.T) {
	t.Parallel()
	repo := &ubStubEventRepo{dayNight: map[int]int64{0: 5, 14: 20}}
	r := gin.New()
	r.GET("/api/v1/user-behavior/day-night", UserBehaviorDayNightHandler(newUBHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/day-night?user_id=1&start_date=2026-07-01&end_date=2026-07-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	pattern := got["pattern"].(map[string]any)
	assert.Len(t, pattern, 24, "24-hour bucket 必须完整")
}

func TestUserBehaviorDayNightHandler_InvalidUserID_Returns400(t *testing.T) {
	t.Parallel()
	repo := &ubStubEventRepo{}
	r := gin.New()
	r.GET("/api/v1/user-behavior/day-night", UserBehaviorDayNightHandler(newUBHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/day-night?user_id=0&start_date=2026-07-01&end_date=2026-07-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// =====================================================
// GET /api/v1/user-behavior/depth
// =====================================================

func TestUserBehaviorDepthHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := &ubStubEventRepo{depth: &repository.InteractionDepth{
		TotalMessages: 42, TotalConversations: 5,
		AvgMessagesPerConv: 8.4, LongestConversationMs: 3600000,
	}}
	r := gin.New()
	r.GET("/api/v1/user-behavior/depth", UserBehaviorDepthHandler(newUBHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/depth?user_id=1&start_date=2026-07-01&end_date=2026-07-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestUserBehaviorDepthHandler_RepoError_Returns500(t *testing.T) {
	t.Parallel()
	repo := &ubStubEventRepo{depthErr: errors.New("postgres timeout")}
	r := gin.New()
	r.GET("/api/v1/user-behavior/depth", UserBehaviorDepthHandler(newUBHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/depth?user_id=1&start_date=2026-07-01&end_date=2026-07-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// =====================================================
// GET /api/v1/user-behavior/frequency
// =====================================================

func TestUserBehaviorFrequencyHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := &ubStubEventRepo{freq: []repository.DailyCount{
		{Date: "2026-07-01", Count: 5},
	}}
	r := gin.New()
	r.GET("/api/v1/user-behavior/frequency", UserBehaviorFrequencyHandler(newUBHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/frequency?user_id=1&start_date=2026-07-01&end_date=2026-07-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestUserBehaviorFrequencyHandler_EmptyCounts_Returns200(t *testing.T) {
	t.Parallel()
	repo := &ubStubEventRepo{freq: nil}
	r := gin.New()
	r.GET("/api/v1/user-behavior/frequency", UserBehaviorFrequencyHandler(newUBHandlerCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/frequency?user_id=1&start_date=2026-07-01&end_date=2026-07-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}