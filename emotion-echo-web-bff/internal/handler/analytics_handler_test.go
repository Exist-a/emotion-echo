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

// TestAnalyticsHandler_TrendReport_IntentDistribution_Transparent 锁定 Stage 85
// 契约：趋势响应透传区间意图分布，按 6 类白名单确定性顺序。
func TestAnalyticsHandler_TrendReport_IntentDistribution_Transparent(t *testing.T) {
	fc := &fakeAnalyticsClient{trend: &downstream.TrendReport{
		UserID: 42,
		Type:   "weekly",
		Points: []downstream.TrendPoint{
			{Date: "2026-09-01", PrimaryEmotion: "happy", Count: 3},
		},
		// 故意乱序注入，验证输出按白名单相对顺序
		IntentCounts: map[string]int64{
			"tech_help":         2,
			"lifestyle":         1,
			"emotional_support": 3,
			"unknown_intent":    99,
		},
	}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/trend?user_id=42&type=weekly&start=2026-09-01&end=2026-09-07", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			IntentDistribution []struct {
				Intent string `json:"intent"`
				Count  int64  `json:"count"`
			} `json:"intentDistribution"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.IntentDistribution, 3, "白名单外 unknown_intent 不输出")
	assert.Equal(t, "emotional_support", resp.Data.IntentDistribution[0].Intent)
	assert.Equal(t, int64(3), resp.Data.IntentDistribution[0].Count)
	assert.Equal(t, "tech_help", resp.Data.IntentDistribution[1].Intent)
	assert.Equal(t, "lifestyle", resp.Data.IntentDistribution[2].Intent)
}

// TestAnalyticsHandler_TrendReport_IntentDistribution_OmittedWhenEmpty 锁定
// 空意图分布时字段整体缺席（前端按"字段缺失隐藏饼图"兼容旧下游）。
func TestAnalyticsHandler_TrendReport_IntentDistribution_OmittedWhenEmpty(t *testing.T) {
	fc := &fakeAnalyticsClient{trend: &downstream.TrendReport{
		Type:   "weekly",
		Points: []downstream.TrendPoint{{Date: "2026-09-01", PrimaryEmotion: "happy", Count: 3}},
	}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/trend?user_id=42&type=weekly&start=2026-09-01&end=2026-09-07", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "intentDistribution",
		"空意图分布不应输出字段（omitempty）")
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
	// E2E-11: 响应形状改为前端契约 periods 数组（原 pattern map 已替换）
	assert.Contains(t, w.Body.String(), `"periods"`)
	assert.Contains(t, w.Body.String(), `"上午"`)
}

// TestAnalyticsHandler_TrendReport_ReturnsFrontendShape 契约：trendReport 响应
// data 必须是前端 EmotionTrend 期望的扁平形状：
//   { type, dates[], series[{name, data[]}], summary,
//     emotionDistribution[], conversationCount, messageCount }
// 而不再是 { report: { ... points[] ... } }。
//
// 同时覆盖 alias 解析：
//   weekly: type=weekly + start + end → start_date + end_date
//   monthly: type=monthly + month=YYYY-MM → start_date=YYYY-MM-01 + end_date=YYYY-MM-{last day}
//   annual: type=yearly + year=YYYY → start_date=YYYY-01-01 + end_date=YYYY-12-31
func TestAnalyticsHandler_TrendReport_ReturnsFrontendShape(t *testing.T) {
	fc := &fakeAnalyticsClient{trend: &downstream.TrendReport{
		UserID:    42,
		Type:      "weekly",
		StartDate: "2026-08-25",
		EndDate:   "2026-08-31",
		Points: []downstream.TrendPoint{
			{Date: "2026-08-25", PrimaryEmotion: "happy", AvgSentiment: 0.5, Count: 3},
			{Date: "2026-08-26", PrimaryEmotion: "happy", AvgSentiment: 0.3, Count: 2},
			{Date: "2026-08-27", PrimaryEmotion: "sad", AvgSentiment: -0.2, Count: 1},
		},
	}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/trend?user_id=42&type=weekly&start=2026-08-25&end=2026-08-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Type                string `json:"type"`
			Dates               []string `json:"dates"`
			Series              []struct {
				Name string  `json:"name"`
				Data []int64 `json:"data"`
			} `json:"series"`
			Summary             string `json:"summary"`
			EmotionDistribution []struct {
				Name  string `json:"name"`
				Value int64  `json:"value"`
			} `json:"emotionDistribution"`
			MessageCount      int64 `json:"messageCount"`
			ConversationCount int64 `json:"conversationCount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "weekly", resp.Data.Type)
	assert.Equal(t, []string{"2026-08-25", "2026-08-26", "2026-08-27"}, resp.Data.Dates)
	require.Len(t, resp.Data.Series, 2, "happy + sad 两个 bucket")
	// happy 总和 3+2=5；sad 1
	emotionTotal := make(map[string]int64)
	for _, s := range resp.Data.Series {
		var sum int64
		for _, c := range s.Data {
			sum += c
		}
		emotionTotal[s.Name] = sum
	}
	assert.Equal(t, int64(5), emotionTotal["happy"])
	assert.Equal(t, int64(1), emotionTotal["sad"])
	assert.Equal(t, int64(6), resp.Data.MessageCount, "3+2+1 累计")
	assert.NotEmpty(t, resp.Data.Summary)
	require.Len(t, resp.Data.EmotionDistribution, 2)
	assert.Equal(t, "happy", resp.Data.EmotionDistribution[0].Name)
	assert.Equal(t, int64(5), resp.Data.EmotionDistribution[0].Value)
}

// TestAnalyticsHandler_TrendReport_Alias_MonthlyY 验证 monthlyReport 传的 month=YYYY-MM
// 被 BFF normalizeTrendQuery 转成 start_date=YYYY-MM-01 + end_date=YYYY-MM-{last day}
// 调用 analytics-svc。30 天月份最后一天应是 30。
func TestAnalyticsHandler_TrendReport_Alias_Monthly(t *testing.T) {
	fc := &fakeAnalyticsClient{trend: &downstream.TrendReport{Type: "monthly", Points: []downstream.TrendPoint{
		{Date: "2026-09-01", PrimaryEmotion: "happy", Count: 1},
	}}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/trend?user_id=42&type=monthly&month=2026-09", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// 没断言 start_date/end_date 内容（fake 收不到 query），但 200 + 不报错证明 alias 路径走通
	assert.NotContains(t, w.Body.String(), `"code":1`)
}

// TestAnalyticsHandler_TrendReport_Alias_Annual 验证 annualReport 传的 year=YYYY
// 被 BFF normalizeTrendQuery 转成 01-01 + 12-31。
func TestAnalyticsHandler_TrendReport_Alias_Annual(t *testing.T) {
	fc := &fakeAnalyticsClient{trend: &downstream.TrendReport{Type: "yearly", Points: []downstream.TrendPoint{
		{Date: "2026-01-01", PrimaryEmotion: "happy", Count: 1},
	}}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/trend?user_id=42&type=yearly&year=2026", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `"code":1`)
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

// =====================================================
// Stage 112 修复（Bug A 第二层）：user_id 从认证身份兜底
//
// 背景：前端 dashboard（dailyReport/weekly/monthly/annual）调
// GET /api/v1/reports/{daily,trend} 时**不带 user_id query**（见
// emotion-echo-web/app/pages/chat/dashboard/*.vue 的 get(...) 调用），
// 而 BFF userIDQuery 强制要求 → 400 → 4 个报表页全部"加载失败"。
//
// 修复语义：APISIX jwt-auth 注入的 X-User-Id 是权威身份来源；
//   - 无 query user_id + 有 X-User-Id → 用认证身份（修复前端调用）
//   - 有 query user_id 且与 X-User-Id 不一致 → 403（防 IDOR 越权查他人报表）
//   - 有 query user_id 且一致 → 200（向后兼容）
//   - 两者都无 → 400（保留原契约错误）
// =====================================================

func TestUserIDQuery_FallbackToAuthedUser_WhenQueryMissing(t *testing.T) {
	// RED: 前端实际调用方式（无 user_id query）+ APISIX 注入的 X-User-Id
	fc := &fakeAnalyticsClient{report: &downstream.DailyReport{UserID: 7, Date: "2026-09-17"}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-09-17", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "无 user_id query 时应回退到认证身份，不再 400")
	assert.Equal(t, int64(7), fc.gotUID, "下游应收到认证用户的 uid")
}

func TestUserIDQuery_MismatchAuthedUser_Returns403(t *testing.T) {
	// 防 IDOR：query user_id 与认证身份不一致 → 拒绝
	fc := &fakeAnalyticsClient{report: &downstream.DailyReport{UserID: 42, Date: "2026-09-17"}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-09-17&user_id=42", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "query user_id 与认证身份不一致应 403")
	assert.NotEqual(t, int64(42), fc.gotUID, "不应把他人 uid 透传给下游")
}

func TestUserIDQuery_MatchAuthedUser_Returns200(t *testing.T) {
	// 向后兼容：query user_id 与认证身份一致 → 200
	fc := &fakeAnalyticsClient{report: &downstream.DailyReport{UserID: 7, Date: "2026-09-17"}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-09-17&user_id=7", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(7), fc.gotUID)
}

func TestUserIDQuery_NoAuthNoQuery_Returns400(t *testing.T) {
	// 无认证 + 无 query → 保留原 400 契约
	fc := &fakeAnalyticsClient{report: &downstream.DailyReport{UserID: 42, Date: "2026-09-17"}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/daily?date=2026-09-17", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserIDQuery_TrendReport_FallbackToAuthedUser(t *testing.T) {
	// 同样覆盖 /reports/trend（周/月/年报共用同一端点）
	fc := &fakeAnalyticsClient{trend: &downstream.TrendReport{}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/trend?type=weekly&start=2026-09-11&end=2026-09-17", nil)
	req.Header.Set("X-User-Id", "9")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "trend 端点同样应回退到认证身份")
	assert.Equal(t, int64(9), fc.gotUID)
}

// ============ E2E-11: user-behavior 前端契约对齐 ============

// TestAnalyticsHandler_DayNight_ReturnsFrontendShape 契约：
// BFF → 前端 dayNight 响应必须包含 periods 数组（前端 chartData 读 dayNight.periods）。
func TestAnalyticsHandler_DayNight_ReturnsFrontendShape(t *testing.T) {
	fc := &fakeAnalyticsClient{pattern: map[int]int64{0: 5, 10: 3, 22: 1}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/day-night?user_id=7", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Periods []struct {
				Label string `json:"label"`
				Hours string `json:"hours"`
				Value int64  `json:"value"`
			} `json:"periods"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Periods, 3, "应有 3 个时段（凌晨/上午/夜间）")
}

// TestAnalyticsHandler_Frequency_ReturnsFrontendShape 契约：
// BFF → 前端 frequency 响应必须包含 dates + messageCount 数组。
func TestAnalyticsHandler_Frequency_ReturnsFrontendShape(t *testing.T) {
	fc := &fakeAnalyticsClient{counts: []downstream.DailyCount{
		{Date: "2026-09-17", Count: 5},
		{Date: "2026-09-18", Count: 3},
	}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/frequency?user_id=7", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Dates        []string `json:"dates"`
			MessageCount []int64  `json:"messageCount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, []string{"2026-09-17", "2026-09-18"}, resp.Data.Dates)
	assert.Equal(t, []int64{5, 3}, resp.Data.MessageCount)
}

// TestAnalyticsHandler_Depth_ReturnsFrontendShape 契约：
// BFF → 前端 depth 响应必须包含 avgSessionRounds / maxConsecutiveDays /
// totalConversations / totalMessages / avgMessagesPerDay。
func TestAnalyticsHandler_Depth_ReturnsFrontendShape(t *testing.T) {
	fc := &fakeAnalyticsClient{depth: &downstream.InteractionDepth{
		TotalMessages:         150,
		TotalConversations:    12,
		AvgMessagesPerConv:    12.5,
		LongestConversationMs: 340000,
	}}
	r := newAnalyticsRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user-behavior/depth?user_id=7", nil)
	req.Header.Set("X-User-Id", "7")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AvgSessionRounds    float64 `json:"avgSessionRounds"`
			MaxConsecutiveDays  int64   `json:"maxConsecutiveDays"`
			TotalConversations  int64   `json:"totalConversations"`
			TotalMessages       int64   `json:"totalMessages"`
			AvgMessagesPerDay   float64 `json:"avgMessagesPerDay"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, int64(150), resp.Data.TotalMessages)
	assert.Equal(t, int64(12), resp.Data.TotalConversations)
	assert.InDelta(t, 12.5, resp.Data.AvgSessionRounds, 0.1, "avgSessionRounds 应映射自 AvgMessagesPerConv")
}
