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
