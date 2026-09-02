// Package handler — analytics_handler_test.go + emotion_query_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.47/52 RED: analytics + emotion query handler 契约测试
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestAnalyticsHandler_DailyReport_ReturnsFrontendShape 契约：dailyReport 响应
// data 必须是前端 DailyReport 期望的扁平形状：
//   { date, summary, emotionDistribution: [{name, value}], conversationCount, messageCount }
// 而不再是 { report: { ... } }（BFF 套的单数 key 会让前端 reportData.* 全是 undefined）。
//
// 历史：stage-30-A 写 BFF 时 OK(c, gin.H{"report": report}) 把数据塞单数 key，
// useApi 把整个 data.data 解出后给前端，前端再读 reportData.summary 拿到 undefined。
// fix/chart-contract-alignment 修复期对齐。
func TestAnalyticsHandler_DailyReport_ReturnsFrontendShape(t *testing.T) {
	fc := &fakeAnalyticsClient{report: &downstream.DailyReport{
		UserID:            42,
		Date:              "2026-08-31",
		MessageCount:      4,
		ConversationCount: 2,
		EmotionCounts:     map[string]int64{"happy": 3, "sad": 1},
		AvgSentiment:      0.4,
	}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?user_id=42&date=2026-08-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Date                string `json:"date"`
			Summary             string `json:"summary"`
			ConversationCount   int64  `json:"conversationCount"`
			MessageCount        int64  `json:"messageCount"`
			EmotionDistribution []struct {
				Name  string `json:"name"`
				Value int64  `json:"value"`
			} `json:"emotionDistribution"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code, "业务码应为 0")
	assert.NotEmpty(t, resp.Data.Summary, "summary 必须有内容（rule-based 模板生成）")
	assert.Equal(t, "2026-08-31", resp.Data.Date)
	assert.Equal(t, int64(2), resp.Data.ConversationCount)
	assert.Equal(t, int64(4), resp.Data.MessageCount)
	require.Len(t, resp.Data.EmotionDistribution, 2, "map → array 必须保留全部条目")
	// happy 比 sad 多，应排前面（确定性）
	assert.Equal(t, "happy", resp.Data.EmotionDistribution[0].Name)
	assert.Equal(t, int64(3), resp.Data.EmotionDistribution[0].Value)
	assert.Equal(t, "sad", resp.Data.EmotionDistribution[1].Name)
	assert.Equal(t, int64(1), resp.Data.EmotionDistribution[1].Value)
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
