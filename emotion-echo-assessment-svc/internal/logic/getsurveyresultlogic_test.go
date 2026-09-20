// Package logic — getsurveyresultlogic_test.go
//
// Sibling test for getsurveyresultlogic.go (per AGENTS.md §1.1).
//
// Extracted from the former survey_logic_test.go umbrella file. Covers:
//   - own result lookup (happy path)
//   - cross-user result isolation (other user's result → ErrNotFound)
//   - no userID → unauthorized
//   - zero result id → validation error
//   - ListMyResults: only returns results for the requesting user
package logic

import (
	"context"
	"fmt"
	"testing"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newGetSurveyResultSvcCtx(repo repository.SurveyRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, SurveyRepo: repo}
}

func TestGetSurveyResultLogic_OwnResult_ReturnsDetail(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx := contextWithUserID(context.Background(), 42)
	submitter := NewSubmitSurveyLogic(ctx, newGetSurveyResultSvcCtx(repo))
	subResp, err := submitter.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 1,
			"q6": 1, "q7": 1, "q8": 1, "q9": 1,
		},
	})
	require.NoError(t, err)

	l := NewGetSurveyResultLogic(ctx, newGetSurveyResultSvcCtx(repo))
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
	submitter := NewSubmitSurveyLogic(ctx42, newGetSurveyResultSvcCtx(repo))
	subResp, err := submitter.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 1,
			"q6": 1, "q7": 1, "q8": 1, "q9": 1,
		},
	})
	require.NoError(t, err)

	ctx99 := contextWithUserID(context.Background(), 99)
	l := NewGetSurveyResultLogic(ctx99, newGetSurveyResultSvcCtx(repo))
	_, err = l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: subResp.ResultID})
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestGetSurveyResultLogic_NoUserID_Unauthorized(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	l := NewGetSurveyResultLogic(context.Background(), newGetSurveyResultSvcCtx(repo))
	_, err := l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestGetSurveyResultLogic_ZeroResultID_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	ctx := contextWithUserID(context.Background(), 1)
	l := NewGetSurveyResultLogic(ctx, newGetSurveyResultSvcCtx(repo))
	_, err := l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "result id is required")
}

func TestListMyResultsLogic_OnlyReturnsOwnResults(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "PHQ-9", Status: 1})

	ctx1 := contextWithUserID(context.Background(), 1)
	sub := NewSubmitSurveyLogic(ctx1, newGetSurveyResultSvcCtx(repo))
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
	sub2 := NewSubmitSurveyLogic(ctx2, newGetSurveyResultSvcCtx(repo))
	_, err = sub2.SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId: 1,
		Answers: map[string]int{
			"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3,
			"q6": 3, "q7": 3, "q8": 3, "q9": 3,
		},
	})
	require.NoError(t, err)

	l := NewGetSurveyResultLogic(ctx1, newGetSurveyResultSvcCtx(repo))
	got, err := l.ListMyResults(&types.ListMyResultsReq{Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, 2, got.Total)
	assert.Len(t, got.Items, 2)
}

// =====================================================
// E2E-14：factorScores 透传
// =====================================================
//
// 人格画像的消费方有两处：① 前端 user 页雷达图 ② BFF 注入 AI system prompt。
// 两者都从"我的结果列表"里挑最新的人格量表结果（riskLevel=="dimension_profile"），
// 因此列表项与详情都必须带 factorScores —— 否则调用方要再发一次详情请求。

// TestGetSurveyResultLogic_IncludesFactorScores 详情必须回传维度分数
func TestGetSurveyResultLogic_IncludesFactorScores(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "BIG5", Status: 1})

	ctx := contextWithUserID(context.Background(), 42)
	sub := NewSubmitSurveyLogic(ctx, newGetSurveyResultSvcCtx(repo))
	ans := map[string]int{}
	for i := 1; i <= 30; i++ {
		ans[fmt.Sprintf("q%d", i)] = 4
	}
	subResp, err := sub.SubmitSurvey(&types.SubmitSurveyReq{SurveyId: 1, Answers: ans})
	require.NoError(t, err)

	l := NewGetSurveyResultLogic(ctx, newGetSurveyResultSvcCtx(repo))
	got, err := l.GetSurveyResult(&types.GetSurveyResultReq{ResultId: subResp.ResultID})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.FactorScores, "结果详情必须带 factorScores")
	assert.InDelta(t, 24.0, got.FactorScores["openness"], 0.001)
}

// TestListMyResultsLogic_IncludesFactorScores 列表项必须回传维度分数
// （BFF 每次对话都靠列表找最新人格画像，列表不带则需二次请求）
func TestListMyResultsLogic_IncludesFactorScores(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemorySurveyRepo()
	repo.Add(&model.Survey{ID: 1, Code: "BIG5", Status: 1})

	ctx := contextWithUserID(context.Background(), 5)
	sub := NewSubmitSurveyLogic(ctx, newGetSurveyResultSvcCtx(repo))
	ans := map[string]int{}
	for i := 1; i <= 30; i++ {
		ans[fmt.Sprintf("q%d", i)] = 2
	}
	_, err := sub.SubmitSurvey(&types.SubmitSurveyReq{SurveyId: 1, Answers: ans})
	require.NoError(t, err)

	l := NewGetSurveyResultLogic(ctx, newGetSurveyResultSvcCtx(repo))
	got, err := l.ListMyResults(&types.ListMyResultsReq{Limit: 20})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	item := got.Items[0]
	assert.Equal(t, "dimension_profile", item.RiskLevel)
	require.NotNil(t, item.FactorScores, "结果列表项必须带 factorScores")
	assert.InDelta(t, 12.0, item.FactorScores["openness"], 0.001)
	assert.Len(t, item.FactorScores, 5)
}