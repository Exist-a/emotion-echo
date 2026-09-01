// Package handler — analytics_handler_test.go + emotion_query_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.47/52 RED: analytics + emotion query handler 契约测试
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeAnalyticsClient 实现 downstream.AnalyticsClient
type fakeAnalyticsClient struct {
	report  *downstream.DailyReport
	trend   *downstream.TrendReport
	pattern map[int]int64
	depth   *downstream.InteractionDepth
	counts  []downstream.DailyCount
	assess  *downstream.MentalAssessment
	err     error
	gotUID  int64
}

func (f *fakeAnalyticsClient) DailyReport(_ context.Context, userID int64, _ string) (*downstream.DailyReport, error) {
	f.gotUID = userID
	return f.report, f.err
}
func (f *fakeAnalyticsClient) TrendReport(_ context.Context, userID int64, _, _, _ string) (*downstream.TrendReport, error) {
	f.gotUID = userID
	return f.trend, f.err
}
func (f *fakeAnalyticsClient) DayNightPattern(_ context.Context, userID int64, _, _ string) (map[int]int64, error) {
	f.gotUID = userID
	return f.pattern, f.err
}
func (f *fakeAnalyticsClient) InteractionDepth(_ context.Context, userID int64, _, _ string) (*downstream.InteractionDepth, error) {
	f.gotUID = userID
	return f.depth, f.err
}
func (f *fakeAnalyticsClient) FrequencyTrend(_ context.Context, userID int64, _, _ string) ([]downstream.DailyCount, error) {
	f.gotUID = userID
	return f.counts, f.err
}
func (f *fakeAnalyticsClient) MentalAssessment(_ context.Context, userID int64, _ string) (*downstream.MentalAssessment, error) {
	f.gotUID = userID
	return f.assess, f.err
}

func newAnalyticsRouter(client downstream.AnalyticsClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&AnalyticsHandler{analytics: client}).Register(r)
	return r
}

func TestAnalyticsHandler_DailyReport_Success(t *testing.T) {
	fc := &fakeAnalyticsClient{report: &downstream.DailyReport{UserID: 42, Date: "2026-08-31", MessageCount: 4}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?user_id=42&date=2026-08-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(42), fc.gotUID, "user_id 应从 query 透传")
	assert.Contains(t, w.Body.String(), `"messageCount":4`)
}

func TestAnalyticsHandler_MissingUserID_Returns400(t *testing.T) {
	r := newAnalyticsRouter(&fakeAnalyticsClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "user_id is required")
}

func TestAnalyticsHandler_DayNight_Success(t *testing.T) {
	fc := &fakeAnalyticsClient{pattern: map[int]int64{9: 2}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/day-night?user_id=42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"pattern"`)
}

func TestAnalyticsHandler_MentalAssessment_Nil_Returns200(t *testing.T) {
	r := newAnalyticsRouter(&fakeAnalyticsClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mental-health/assessment?user_id=42&type=daily", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"assessment":null`, "无评估时 assessment 应为 null")
}

// ===================== emotion query =====================

// fakeEmotionQueryHandlerClient 实现 downstream.EmotionQueryClient
type fakeEmotionQueryHandlerClient struct {
	emotion *emotionquery.Emotion
	list    []*emotionquery.Emotion
	total   int32
	err     error
	// Stage 34: fused 端点（analytics handler 不需要，但接口要满足）
	fused    *emotionquery.FusedEmotion
	fusedErr error
}

func (f *fakeEmotionQueryHandlerClient) ByMessage(_ context.Context, _ int64) (*emotionquery.Emotion, error) {
	return f.emotion, f.err
}
func (f *fakeEmotionQueryHandlerClient) ByConversation(_ context.Context, _ int64, _ int) ([]*emotionquery.Emotion, int32, error) {
	return f.list, f.total, f.err
}
func (f *fakeEmotionQueryHandlerClient) ByFusedMessage(_ context.Context, _ int64) (*emotionquery.FusedEmotion, error) {
	return f.fused, f.fusedErr
}

func newEmotionQueryRouter(client downstream.EmotionQueryClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&EmotionQueryHandler{query: client}).Register(r)
	return r
}

func TestEmotionQueryHandler_ByMessage_Success(t *testing.T) {
	r := newEmotionQueryRouter(&fakeEmotionQueryHandlerClient{
		emotion: &emotionquery.Emotion{MessageId: 42, PrimaryEmotion: "happy"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"happy"`)
}

func TestEmotionQueryHandler_ByConversation_Success(t *testing.T) {
	r := newEmotionQueryRouter(&fakeEmotionQueryHandlerClient{
		list:  []*emotionquery.Emotion{{MessageId: 1, PrimaryEmotion: "calm"}},
		total: 1,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/conversation/10?limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"total":1`)
}

func TestEmotionQueryHandler_InvalidID_Returns400(t *testing.T) {
	r := newEmotionQueryRouter(&fakeEmotionQueryHandlerClient{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
