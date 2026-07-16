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

func newSurveySvcCtx(repo repository.SurveyRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, SurveyRepo: repo}
}

func contextWithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, middleware.CtxUserIDKey{}, uid)
}

// =====================================================
// ListSurveysLogic 测试
// =====================================================

func TestListSurveysLogic_Empty_ReturnsEmptyItems(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewListSurveysLogic(context.Background(), newSurveySvcCtx(repo))

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

	l := NewListSurveysLogic(context.Background(), newSurveySvcCtx(repo))
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
	l := NewListSurveysLogic(context.Background(), newSurveySvcCtx(repo))
	resp, err := l.ListSurveys(&types.ListSurveysReq{Limit: 50})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, 9, resp.Items[0].QuestionNum)
}

// =====================================================
// GetSurveyLogic 测试
// =====================================================

func TestGetSurveyLogic_Existing_ReturnsQuestions(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{
		ID: 5, Code: "GAD-7", Title: "anxiety", Status: 1,
		Questions: model.JSONMap{"items": []any{"q1", "q2"}},
	})
	l := NewGetSurveyLogic(context.Background(), newSurveySvcCtx(repo))
	resp, err := l.GetSurvey(&types.GetSurveyReq{Id: 5})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "GAD-7", resp.Code)
	assert.Equal(t, "anxiety", resp.Title)
}

func TestGetSurveyLogic_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewGetSurveyLogic(context.Background(), newSurveySvcCtx(repo))
	_, err := l.GetSurvey(&types.GetSurveyReq{Id: 999})
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestGetSurveyLogic_ZeroID_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewGetSurveyLogic(context.Background(), newSurveySvcCtx(repo))
	_, err := l.GetSurvey(&types.GetSurveyReq{Id: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "survey id is required")
}

// =====================================================
// SubmitSurveyLogic 测试（使用 scoring 包真实计分）
// =====================================================

func TestSubmitSurveyLogic_HappyPath_PHQ9_CalculatesScore(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx := contextWithUserID(context.Background(), 42)
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

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
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

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
	l := NewSubmitSurveyLogic(context.Background(), newSurveySvcCtx(repo))

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
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

	_, err := l.SubmitSurvey(&types.SubmitSurveyReq{SurveyId: 1, Answers: map[string]int{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "answers cannot be empty")
}

func TestSubmitSurveyLogic_PHQ9_OutOfRange_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})
	ctx := contextWithUserID(context.Background(), 1)
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

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
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

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
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

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
	l := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))

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

// =====================================================
// GetSurveyResultLogic 测试
// =====================================================

func TestGetSurveyResultLogic_OwnResult_ReturnsDetail(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx := contextWithUserID(context.Background(), 42)
	submitter := NewSubmitSurveyLogic(ctx, newSurveySvcCtx(repo))
	subResp, err := submitter.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 1,
			"q6": 1, "q7": 1, "q8": 1, "q9": 1,
		},
	})
	require.NoError(t, err)

	l := NewGetSurveyResultLogic(ctx, newSurveySvcCtx(repo))
	got, err := l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: subResp.ResultID})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, subResp.ResultID, got.ResultID)
	assert.Equal(t, int64(42), got.UserID)
	assert.InDelta(t, 9.0, got.TotalScore, 0.001)
}

func TestGetSurveyResultLogic_OtherUserResult_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx42 := contextWithUserID(context.Background(), 42)
	submitter := NewSubmitSurveyLogic(ctx42, newSurveySvcCtx(repo))
	subResp, err := submitter.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 1,
			"q6": 1, "q7": 1, "q8": 1, "q9": 1,
		},
	})
	require.NoError(t, err)

	ctx99 := contextWithUserID(context.Background(), 99)
	l := NewGetSurveyResultLogic(ctx99, newSurveySvcCtx(repo))
	_, err = l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: subResp.ResultID})
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestGetSurveyResultLogic_NoUserID_Unauthorized(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewGetSurveyResultLogic(context.Background(), newSurveySvcCtx(repo))
	_, err := l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestGetSurveyResultLogic_ZeroResultID_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	ctx := contextWithUserID(context.Background(), 1)
	l := NewGetSurveyResultLogic(ctx, newSurveySvcCtx(repo))
	_, err := l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "result id is required")
}

func TestListMyResultsLogic_OnlyReturnsOwnResults(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx1 := contextWithUserID(context.Background(), 1)
	sub := NewSubmitSurveyLogic(ctx1, newSurveySvcCtx(repo))
	_, err := sub.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 1,
			"q6": 1, "q7": 1, "q8": 1, "q9": 1,
		},
	})
	require.NoError(t, err)
	_, err = sub.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 2, "q2": 2, "q3": 2, "q4": 2, "q5": 2,
			"q6": 2, "q7": 2, "q8": 2, "q9": 2,
		},
	})
	require.NoError(t, err)

	ctx2 := contextWithUserID(context.Background(), 2)
	sub2 := NewSubmitSurveyLogic(ctx2, newSurveySvcCtx(repo))
	_, err = sub2.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3,
			"q6": 3, "q7": 3, "q8": 3, "q9": 3,
		},
	})
	require.NoError(t, err)

	l := NewGetSurveyResultLogic(ctx1, newSurveySvcCtx(repo))
	got, err := l.ListMyResults(&types.ListMyResultsReq{Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, 2, got.Total)
	assert.Len(t, got.Items, 2)
}