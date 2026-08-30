// Package logic — submitsurveylogic_test.go
//
// Sibling test for submitsurveylogic.go (per AGENTS.md §1.1).
//
// Extracted from the former survey_logic_test.go umbrella file. Covers:
//   - happy path PHQ-9 / PSQI scoring (real scoring package)
//   - no userID in ctx → unauthorized
//   - empty answers / out-of-range answer / wrong answer count → validation
//   - unknown survey id → repository.ErrNotFound
//   - PHQ-9 extreme-level (all 3s) → total=27, "extreme" risk
package logic

import (
	"context"
	"testing"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/middleware"
	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSubmitSurveySvcCtx(repo repository.SurveyRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, SurveyRepo: repo}
}

func contextWithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, middleware.CtxUserIDKey{}, uid)
}

func TestSubmitSurveyLogic_HappyPath_PHQ9_CalculatesScore(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx := contextWithUserID(context.Background(), 42)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	// PHQ-9：9 题 0-3，本例 total=11（moderate 边界）
	resp, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 2, "q2": 1, "q3": 2, "q4": 1, "q5": 2,
			"q6": 1, "q7": 1, "q8": 1, "q9": 0,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint64(1), resp.SurveyID)
	assert.Equal(t, 9, resp.Answered)
	assert.InDelta(t, 11.0, resp.TotalScore, 0.001)
	assert.Equal(t, "moderate", resp.RiskLevel)
}

func TestSubmitSurveyLogic_HappyPath_PSQI_CalculatesComponents(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PSQI", Status: 1})

	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	// PSQI：7 个 component 0-3，本例 total=6（mild 边界）
	resp, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"C1": 1, "C2": 1, "C3": 1, "C4": 1, "C5": 1, "C6": 0, "C7": 1,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "mild", resp.RiskLevel)
	assert.InDelta(t, 6.0, resp.TotalScore, 0.001)
}

func TestSubmitSurveyLogic_NoUserID_Unauthorized(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Status: 1})
	l := NewSubmitSurveyLogic(context.Background(), newSubmitSurveySvcCtx(repo))

	_, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers:  map[string]int{"q1": 1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestSubmitSurveyLogic_EmptyAnswers_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Status: 1})
	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	_, err := l.SubmitSurvey(&types.SubmitSurveyReq{SurveyId: 1, Answers: map[string]int{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "answers cannot be empty")
}

func TestSubmitSurveyLogic_PHQ9_OutOfRange_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})
	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	// PHQ-9 单题超过 3
	_, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 99, "q2": 1, "q3": 1, "q4": 1, "q5": 1,
			"q6": 1, "q7": 1, "q8": 1, "q9": 1,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0-3")
}

func TestSubmitSurveyLogic_PHQ9_WrongAnswerCount_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})
	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	// PHQ-9 必须 9 题，这里给 5 题
	_, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers:  map[string]int{"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 1},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "9 answers")
}

func TestSubmitSurveyLogic_SurveyNotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	_, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 999,
		Answers:  map[string]int{"q1": 1},
	})
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestSubmitSurveyLogic_PHQ9_ExtremeLevel_AllThrees(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})
	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSubmitSurveySvcCtx(repo))

	resp, err := l.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3,
			"q6": 3, "q7": 3, "q8": 3, "q9": 3,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "extreme", resp.RiskLevel)
	assert.InDelta(t, 27.0, resp.TotalScore, 0.001)
}