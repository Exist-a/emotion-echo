// Package handler — survey_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.42 RED: survey handler 契约测试
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAssessmentClient 实现 downstream.AssessmentClient
type fakeAssessmentClient struct {
	items    []downstream.SurveyItem
	total    int
	detail   *downstream.SurveyDetail
	submit   *downstream.SubmitSurveyResp
	results  []downstream.SurveyResultItem
	rTotal   int
	result   *downstream.SurveyResultDetail
	err      error
}

func (f *fakeAssessmentClient) ListSurveys(_ context.Context, _ int) ([]downstream.SurveyItem, int, error) {
	return f.items, f.total, f.err
}
func (f *fakeAssessmentClient) GetSurvey(_ context.Context, _ uint64) (*downstream.SurveyDetail, error) {
	return f.detail, f.err
}
func (f *fakeAssessmentClient) SubmitSurvey(_ context.Context, _ uint64, _ downstream.SubmitSurveyReq) (*downstream.SubmitSurveyResp, error) {
	return f.submit, f.err
}
func (f *fakeAssessmentClient) ListResults(_ context.Context, _ int) ([]downstream.SurveyResultItem, int, error) {
	return f.results, f.rTotal, f.err
}
func (f *fakeAssessmentClient) GetResult(_ context.Context, _ uint64) (*downstream.SurveyResultDetail, error) {
	return f.result, f.err
}

func newSurveyRouter(client downstream.AssessmentClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&SurveyHandler{assessment: client}).Register(r)
	return r
}

func TestSurveyHandler_ListSurveys_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{
		items: []downstream.SurveyItem{{ID: 1, Code: "SDS", Title: "抑郁量表"}},
		total: 1,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys?limit=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"items"`)
	assert.Contains(t, w.Body.String(), `"SDS"`)
}

func TestSurveyHandler_GetSurvey_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{detail: &downstream.SurveyDetail{
		ID: 1, Code: "SDS", Title: "抑郁量表", Category: "抑郁", Version: 1,
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"SDS"`)
}

func TestSurveyHandler_SubmitSurvey_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{submit: &downstream.SubmitSurveyResp{
		ResultID: 9, SurveyID: 1, TotalScore: 42.5, Answered: 20, RiskLevel: "moderate",
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit",
		bytes.NewReader([]byte(`{"answers":{"q1":3,"q2":2}}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"moderate"`)
}

func TestSurveyHandler_SubmitSurvey_NoAnswers_Returns400(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit",
		bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "answers is required")
}

func TestSurveyHandler_ListResults_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{
		results: []downstream.SurveyResultItem{{ResultID: 9, SurveyID: 1, RiskLevel: "low"}},
		rTotal:  1,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results?limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"low"`)
}

func TestSurveyHandler_GetResult_Success(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{result: &downstream.SurveyResultDetail{
		ResultID: 9, SurveyID: 1, UserID: 7, TotalScore: 42.5, RiskLevel: "moderate",
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results/9", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"userId":7`)
}

func TestSurveyHandler_GetSurvey_NotFound_Returns404(t *testing.T) {
	r := newSurveyRouter(&fakeAssessmentClient{err: &downstream.APIError{StatusCode: http.StatusNotFound, Msg: "survey not found"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "survey not found")
}

// ==================== HTTP 绕过路径契约测试 ====================
// E2E-13: 当 assessmentBase 被设置时，handler 走 HTTP 直取而非 gRPC

// newSurveyRouterWithHTTP 创建带 HTTP 绕过的 survey handler 路由
func newSurveyRouterWithHTTP(fakeAssessment *httptest.Server) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewSurveyHandler(&fakeAssessmentClient{})
	h.WithAssessmentBase(fakeAssessment.URL)
	h.Register(r)
	return r
}

func TestSurveyHandler_GetSurveyHTTP_QuestionsAsArray(t *testing.T) {
	// 模拟 assessment-svc 返回 questions 为 map（JSONB 原始格式）
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":1,"code":"PHQ-9","title":"PHQ-9","category":"depression","version":1,
			"questions":{
				"q1":{"title":"兴趣减退","type":"radio","options":[{"id":1,"text":"没有","score":0},{"id":2,"text":"有","score":1}]},
				"q2":{"title":"心情低落","type":"radio","options":[{"id":1,"text":"没有","score":0},{"id":2,"text":"有","score":1}]}
			}
		}`))
	}))
	defer fake.Close()

	r := newSurveyRouterWithHTTP(fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// questions 应为数组（非 map）
	assert.Contains(t, body, `"questions":[`)
	// 每个 question 应有 id 字段（从 map key 注入）
	assert.Contains(t, body, `"id":"q1"`)
	assert.Contains(t, body, `"id":"q2"`)
	// 响应应被 BFF 包装为 {code:0, data:{...}}
	assert.Contains(t, body, `"code":0`)
}

func TestSurveyHandler_SubmitSurveyHTTP_PreservesAnswerKeys(t *testing.T) {
	// 模拟 assessment-svc，验证收到的 answers key 是 "q1"/"q2"（非数字）
	var receivedBody string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		receivedBody = buf.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"resultId":1,"surveyId":1,"totalScore":2,"answered":2,"riskLevel":"none"}`))
	}))
	defer fake.Close()

	r := newSurveyRouterWithHTTP(fake)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit",
		bytes.NewReader([]byte(`{"answers":{"q1":1,"q2":1}}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// 验证转发给 assessment-svc 的 body 保留了 "q1"/"q2" key
	assert.Contains(t, receivedBody, `"q1"`)
	assert.Contains(t, receivedBody, `"q2"`)
	assert.NotContains(t, receivedBody, `"1":`) // 不应有数字 key
}

func TestSurveyHandler_GetResultHTTP_ReturnsRiskLevel(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"resultId":1,"surveyId":1,"userId":48,"totalScore":12,"riskLevel":"moderate","durationSec":60,"answers":{"q1":1},"submittedAt":1789878019}`))
	}))
	defer fake.Close()

	r := newSurveyRouterWithHTTP(fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"riskLevel":"moderate"`)
	assert.Contains(t, body, `"totalScore":12`)
	// 响应应被 BFF 包装
	assert.Contains(t, body, `"code":0`)
}

func TestSurveyHandler_GetSurveyHTTP_QuestionsSortedByKey(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":1,"code":"TEST","title":"Test","category":"test","version":1,
			"questions":{
				"q3":{"title":"C","type":"radio","options":[]},
				"q1":{"title":"A","type":"radio","options":[]},
				"q2":{"title":"B","type":"radio","options":[]}
			}
		}`))
	}))
	defer fake.Close()

	r := newSurveyRouterWithHTTP(fake)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	idx1 := bytes.Index(w.Body.Bytes(), []byte(`"id":"q1"`))
	idx2 := bytes.Index(w.Body.Bytes(), []byte(`"id":"q2"`))
	idx3 := bytes.Index(w.Body.Bytes(), []byte(`"id":"q3"`))
	assert.True(t, idx1 < idx2, "q1 should come before q2")
	assert.True(t, idx2 < idx3, "q2 should come before q3")
}
