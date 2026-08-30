// Package logic — listsurveyslogic_test.go
//
// Sibling test for listsurveyslogic.go (per AGENTS.md §1.1).
//
// Extracted from the former survey_logic_test.go umbrella file. Covers
// the ListSurveys logic contract:
//   - empty repo → empty Items / Total=0
//   - only Status=1 (active) surveys are returned; Status=0 filtered out
//   - countQuestions() extracts len from JSONMap's "items" array
package logic

import (
	"context"
	"testing"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newListSurveysSvcCtx(repo repository.SurveyRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, SurveyRepo: repo}
}

func TestListSurveysLogic_Empty_ReturnsEmptyItems(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewListSurveysLogic(context.Background(), newListSurveysSvcCtx(repo))

	resp, err := l.ListSurveys(&types.ListSurveysReq{Limit: 50})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Items)
	assert.Equal(t, 0, resp.Total)
}

func TestListSurveysLogic_ReturnsActiveSurveysOnly(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Title: "depression", Status: 1})
	repo.Add(&model.Survey{ID: 2, Code: "GAD-7", Title: "anxiety", Status: 1})
	repo.Add(&model.Survey{ID: 3, Code: "OLD", Title: "deprecated", Status: 0})

	l := NewListSurveysLogic(context.Background(), newListSurveysSvcCtx(repo))
	resp, err := l.ListSurveys(&types.ListSurveysReq{Limit: 50})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, 2, resp.Total)
}

func TestListSurveysLogic_CountQuestionsFromItemsArray(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{
		ID: 1, Code: "PHQ-9", Title: "depression", Status: 1,
		Questions: model.JSONMap{
			"items": []any{"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8", "q9"},
		},
	})
	l := NewListSurveysLogic(context.Background(), newListSurveysSvcCtx(repo))
	resp, err := l.ListSurveys(&types.ListSurveysReq{Limit: 50})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, 9, resp.Items[0].QuestionNum)
}