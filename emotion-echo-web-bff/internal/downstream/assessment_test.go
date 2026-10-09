// Package downstream — assessment_test.go
//
// Stage 30 / stage-30-web-bff.md T2.18-20 RED: AssessmentClient 契约测试
package downstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssessmentClient_ListSurveys_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/surveys", r.URL.Path)
		assert.Equal(t, "50", r.URL.Query().Get("limit"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []SurveyItem{{ID: 1, Code: "SDS", Title: "抑郁量表", Category: "抑郁", QuestionNum: 20, Version: 1}},
			"total": 1,
		})
	}))
	defer srv.Close()

	c := NewAssessmentClient(AssessmentClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	items, total, err := c.ListSurveys(context.Background(), 50)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "SDS", items[0].Code)
}

func TestAssessmentClient_GetSurvey_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/surveys/1", r.URL.Path)
		// E2E-31：assessment-svc 侧 questions 是 JSONB **map**（键 "q1"…），
		// 且故意逆序给出，验证客户端归一化时确实按键排序（而非依赖 map 遍历顺序）。
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 1, "code": "PHQ-9", "title": "抑郁症筛查量表",
			"description": "过去两周内，以下问题困扰你的频率是多少？",
			"questions": map[string]any{
				"q2": map[string]any{
					"title":   "感到心情低落",
					"options": []any{map[string]any{"id": 1, "text": "完全没有", "score": 0}},
				},
				"q1": map[string]any{
					"title":   "做事时提不起劲或没有兴趣",
					"options": []any{map[string]any{"id": 4, "text": "几乎每天", "score": 3}},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewAssessmentClient(AssessmentClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	s, err := c.GetSurvey(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "PHQ-9", s.Code)
	assert.Equal(t, "过去两周内，以下问题困扰你的频率是多少？", s.Description)
	// 前端契约：questions 是**有序数组**，每题 id = JSONB 键（不是数字下标）
	require.Len(t, s.Questions, 2)
	assert.Equal(t, "q1", s.Questions[0]["id"])
	assert.Equal(t, "q2", s.Questions[1]["id"])
	assert.Equal(t, "做事时提不起劲或没有兴趣", s.Questions[0]["title"])
	// 选项必须是 {id,text,score} 结构（前端 `opt.score` 计分）
	opts, ok := s.Questions[0]["options"].([]any)
	require.True(t, ok, "options 应为数组，实际 %T", s.Questions[0]["options"])
	require.Len(t, opts, 1)
	assert.Equal(t, float64(3), opts[0].(map[string]any)["score"])
}

func TestAssessmentClient_SubmitSurvey_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/surveys/1/submit", r.URL.Path)

		var req SubmitSurveyReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, 3, req.Answers["q1"])

		_ = json.NewEncoder(w).Encode(SubmitSurveyResp{
			ResultID: 9, SurveyID: 1, TotalScore: 42.5, Answered: 20, RiskLevel: "moderate",
		})
	}))
	defer srv.Close()

	c := NewAssessmentClient(AssessmentClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	resp, err := c.SubmitSurvey(context.Background(), 1, SubmitSurveyReq{Answers: map[string]int{"q1": 3}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint64(9), resp.ResultID)
	assert.Equal(t, "moderate", resp.RiskLevel)
}

func TestAssessmentClient_ListResults_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/surveys/results", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []SurveyResultItem{{ResultID: 9, SurveyID: 1, TotalScore: 42.5, RiskLevel: "moderate", SubmittedAt: 123}},
			"total": 1,
		})
	}))
	defer srv.Close()

	c := NewAssessmentClient(AssessmentClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	items, total, err := c.ListResults(context.Background(), 20)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "moderate", items[0].RiskLevel)
}

func TestAssessmentClient_GetResult_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/surveys/results/9", r.URL.Path)
		_ = json.NewEncoder(w).Encode(SurveyResultDetail{
			ResultID: 9, SurveyID: 1, UserID: 7, TotalScore: 42.5, RiskLevel: "moderate",
			DurationSec: 120, SubmittedAt: 123,
		})
	}))
	defer srv.Close()

	c := NewAssessmentClient(AssessmentClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	r, err := c.GetResult(context.Background(), 9)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, int64(7), r.UserID)
	assert.Equal(t, 120, r.DurationSec)
}

func TestAssessmentClient_NotFound_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "survey not found"})
	}))
	defer srv.Close()

	c := NewAssessmentClient(AssessmentClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.GetSurvey(context.Background(), 999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "survey not found")
}
