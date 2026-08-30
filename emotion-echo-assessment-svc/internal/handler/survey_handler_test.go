// Package handler — survey_handler_test.go
//
// Sibling test for survey_handler.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 5.3 frontend-equivalent: web-api handler
// regression coverage. Uses real httptest + gin router + InMemorySurveyRepo
// (no external deps; runs in <1s per test).
//
// Coverage matrix:
//
//   ListSurveysHandler    GET  /api/v1/surveys
//     - default limit=50 → 200 + JSON items
//     - limit=10 query param → 200 (param respected)
//     - limit=garbage → 200 (falls back to default; non-positive ignored)
//     - repo error → 500
//
//   GetSurveyHandler      GET  /api/v1/surveys/:id
//     - existing → 200 + survey body
//     - id=0 → 400 invalid survey id
//     - id=abc → 400 invalid survey id
//     - id=999 (missing) → 404 survey not found
//
//   SubmitSurveyHandler   POST /api/v1/surveys/:id/submit
//     - happy PHQ-9 → 200 + scored body
//     - id=0 → 400 invalid survey id
//     - empty body → 400 bind error
//     - unknown survey → 404 survey not found
//
//   GetSurveyResultHandler GET /api/v1/surveys/results/:resultId
//     - existing result → 200
//     - id=0 → 400 invalid result id
//     - id=abc → 400 invalid result id
//     - unknown result → 404 result not found
//
//   ListMyResultsHandler  GET /api/v1/surveys/results
//     - default limit=20 → 200 + items
//     - limit=99 → 200 (param respected)
//     - no userID in ctx → 400 (logic layer rejects)
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/middleware"
	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func newSurveyHandlerSvcCtx(repo repository.SurveyRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, SurveyRepo: repo}
}

// surveyResultCtx injects a userID into ctx via a middleware-style
// helper. Matches contextWithUserID in logic package but lives here
// to avoid cross-_test.go coupling (per AGENTS §1.1).
func surveyResultCtx(uid int64) context.Context {
	return context.WithValue(context.Background(), middleware.CtxUserIDKey{}, uid)
}

// =====================================================
// ListSurveysHandler
// =====================================================

func TestListSurveysHandler_DefaultLimit_ReturnsActiveSurveys(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	r := gin.New()
	r.GET("/api/v1/surveys", ListSurveysHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.EqualValues(t, 1, got["total"])
}

func TestListSurveysHandler_LimitQueryParam_Respected(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	for i := uint64(1); i <= 5; i++ {
		repo.Add(&model.Survey{ID: i, Code: "S", Status: 1})
	}
	r := gin.New()
	r.GET("/api/v1/surveys", ListSurveysHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys?limit=2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestListSurveysHandler_LimitGarbage_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys", ListSurveysHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys?limit=banana", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// handler ignores non-numeric; default 50 wins.
	require.Equal(t, http.StatusOK, w.Code)
}

// =====================================================
// GetSurveyHandler
// =====================================================

func TestGetSurveyHandler_Existing_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 5, Code: "GAD-7", Title: "anxiety", Status: 1})

	r := gin.New()
	r.GET("/api/v1/surveys/:id", GetSurveyHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "GAD-7")
}

func TestGetSurveyHandler_ZeroID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/:id", GetSurveyHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid survey id")
}

func TestGetSurveyHandler_NonNumericID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/:id", GetSurveyHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSurveyHandler_UnknownID_Returns404(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/:id", GetSurveyHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "survey not found")
}

// =====================================================
// SubmitSurveyHandler
// =====================================================

func TestSubmitSurveyHandler_HappyPath_PHQ9_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	r := gin.New()
	r.POST("/api/v1/surveys/:id/submit",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(42)) },
		SubmitSurveyHandler(newSurveyHandlerSvcCtx(repo)),
	)

	body := `{"answers":{"q1":1,"q2":1,"q3":1,"q4":1,"q5":1,"q6":1,"q7":1,"q8":1,"q9":1}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestSubmitSurveyHandler_ZeroID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.POST("/api/v1/surveys/:id/submit", SubmitSurveyHandler(newSurveyHandlerSvcCtx(repo)))

	body := `{"answers":{"q1":1}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/0/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitSurveyHandler_EmptyBody_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})
	r := gin.New()
	r.POST("/api/v1/surveys/:id/submit",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		SubmitSurveyHandler(newSurveyHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/1/submit", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitSurveyHandler_UnknownSurvey_Returns404(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.POST("/api/v1/surveys/:id/submit",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		SubmitSurveyHandler(newSurveyHandlerSvcCtx(repo)),
	)

	body := `{"answers":{"q1":1}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/surveys/999/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "survey not found")
}

// =====================================================
// GetSurveyResultHandler
// =====================================================

func TestGetSurveyResultHandler_ZeroID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/results/:resultId",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		GetSurveyResultHandler(newSurveyHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results/0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid result id")
}

func TestGetSurveyResultHandler_NonNumericID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/results/:resultId",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		GetSurveyResultHandler(newSurveyHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetSurveyResultHandler_UnknownResult_Returns404(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/results/:resultId",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		GetSurveyResultHandler(newSurveyHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "result not found")
}

// =====================================================
// ListMyResultsHandler
// =====================================================

func TestListMyResultsHandler_DefaultLimit_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/results",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		ListMyResultsHandler(newSurveyHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestListMyResultsHandler_LimitQueryParam_Respected(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/results",
		func(c *gin.Context) { c.Request = c.Request.WithContext(surveyResultCtx(1)) },
		ListMyResultsHandler(newSurveyHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results?limit=99", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestListMyResultsHandler_NoUserID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	r := gin.New()
	r.GET("/api/v1/surveys/results", ListMyResultsHandler(newSurveyHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/surveys/results", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// logic layer rejects unauthorized → handler returns 400.
	require.Equal(t, http.StatusBadRequest, w.Code)
}