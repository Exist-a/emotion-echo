// Package logic — getsurveylogic_test.go
//
// Sibling test for getsurveylogic.go (per AGENTS.md §1.1).
//
// Extracted from the former survey_logic_test.go umbrella file. Covers:
//   - happy path: existing survey returns its questions/code/title
//   - not found: unknown id returns repository.ErrNotFound
//   - validation: zero id rejected before repo call
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

func newGetSurveySvcCtx(repo repository.SurveyRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, SurveyRepo: repo}
}

func TestGetSurveyLogic_Existing_ReturnsQuestions(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{
		ID: 5, Code: "GAD-7", Title: "anxiety", Status: 1,
		Questions: model.JSONMap{"items": []any{"q1", "q2"}},
	})
	l := NewGetSurveyLogic(context.Background(), newGetSurveySvcCtx(repo))
	resp, err := l.GetSurvey(&types.GetSurveyReq{Id: 5})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "GAD-7", resp.Code)
	assert.Equal(t, "anxiety", resp.Title)
}

func TestGetSurveyLogic_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewGetSurveyLogic(context.Background(), newGetSurveySvcCtx(repo))
	_, err := l.GetSurvey(&types.GetSurveyReq{Id: 999})
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestGetSurveyLogic_ZeroID_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewGetSurveyLogic(context.Background(), newGetSurveySvcCtx(repo))
	_, err := l.GetSurvey(&types.GetSurveyReq{Id: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "survey id is required")
}