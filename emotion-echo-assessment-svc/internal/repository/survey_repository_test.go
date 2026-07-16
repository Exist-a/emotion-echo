package repository

import (
	"context"
	"testing"

	"emotion-echo-assessment-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSurveyRepo_InMemory_GetByID_Existing(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "SCL-90", Title: "症状自评量表"})

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "SCL-90", got.Code)
	assert.Equal(t, "症状自评量表", got.Title)
}

func TestSurveyRepo_InMemory_GetByID_NotFound_ReturnsNil(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	got, err := repo.GetByID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSurveyRepo_InMemory_GetByCode_Existing(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 2, Code: "SAS", Title: "焦虑自评量表"})

	got, err := repo.GetByCode(context.Background(), "SAS")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(2), got.ID)
}

func TestSurveyRepo_InMemory_List_ReturnsAll(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "SCL-90"})
	repo.Add(&model.Survey{ID: 2, Code: "SAS"})

	out, err := repo.List(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, out, 2)
}

func TestSurveyRepo_InMemory_Ping_OK(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	err := repo.Ping(context.Background())
	require.NoError(t, err)
}

func TestSurveyRepo_InMemory_SaveResult_AssignsID(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	res := &model.SurveyResult{
		UserID:     1,
		SurveyID:   1,
		TotalScore: 15.0,
		RiskLevel:  "medium",
	}

	require.NoError(t, repo.SaveResult(context.Background(), res))
	assert.NotZero(t, res.ID, "SaveResult should assign a non-zero ID")
}

func TestSurveyRepo_InMemory_SaveResult_PreservesProvidedID(t *testing.T) {
	t.Parallel()

	repo := NewInMemorySurveyRepo()
	res := &model.SurveyResult{
		ID:         100,
		UserID:     1,
		SurveyID:   1,
		TotalScore: 15.0,
	}
	require.NoError(t, repo.SaveResult(context.Background(), res))
	assert.Equal(t, uint64(100), res.ID)
}