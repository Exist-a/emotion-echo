// Package downstream — personality_test.go
//
// E2E-14：从"我的量表结果"里挑出最新的人格画像。
//
// 判据是 riskLevel == "dimension_profile" —— 这是 BigFiveScorer 写入的固定标记
// （人格量表没有"风险"概念，用该值区分于症状量表的 none/mild/...）。
package downstream

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAssessmentForPersonality 只实现 ListResults，其余 panic（本测试不触及）
type fakeAssessmentForPersonality struct {
	items []SurveyResultItem
	err   error
}

func (f *fakeAssessmentForPersonality) ListSurveys(context.Context, int) ([]SurveyItem, int, error) {
	panic("not used")
}
func (f *fakeAssessmentForPersonality) GetSurvey(context.Context, uint64) (*SurveyDetail, error) {
	panic("not used")
}
func (f *fakeAssessmentForPersonality) SubmitSurvey(context.Context, uint64, SubmitSurveyReq) (*SubmitSurveyResp, error) {
	panic("not used")
}
func (f *fakeAssessmentForPersonality) ListResults(context.Context, int) ([]SurveyResultItem, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.items, len(f.items), nil
}
func (f *fakeAssessmentForPersonality) GetResult(context.Context, uint64) (*SurveyResultDetail, error) {
	panic("not used")
}

func TestPersonalityProfileSource_PicksLatestPersonalityResult(t *testing.T) {
	t.Parallel()
	dims := map[string]float64{"openness": 26, "neuroticism": 10}
	fake := &fakeAssessmentForPersonality{items: []SurveyResultItem{
		// 最新的是一条症状量表结果 → 必须跳过
		{ResultID: 9, RiskLevel: "moderate", SubmittedAt: 300},
		// 人格量表结果（较旧）
		{ResultID: 8, RiskLevel: "dimension_profile", SubmittedAt: 200, FactorScores: dims},
		{ResultID: 7, RiskLevel: "dimension_profile", SubmittedAt: 100,
			FactorScores: map[string]float64{"openness": 6}},
	}}
	src := NewPersonalityProfileSource(fake)

	got, err := src.LatestPersonalityProfile(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.InDelta(t, 26.0, got["openness"], 0.001, "取最新的那条人格结果（submittedAt 最大）")
	assert.InDelta(t, 10.0, got["neuroticism"], 0.001)
}

func TestPersonalityProfileSource_NoPersonalityResult_ReturnsNil(t *testing.T) {
	t.Parallel()
	fake := &fakeAssessmentForPersonality{items: []SurveyResultItem{
		{ResultID: 1, RiskLevel: "mild", SubmittedAt: 100},
		{ResultID: 2, RiskLevel: "none", SubmittedAt: 200},
	}}
	src := NewPersonalityProfileSource(fake)

	got, err := src.LatestPersonalityProfile(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got, "无（可用的）人格结果必须返回 nil，交由调用方回落基础 prompt")
}

func TestPersonalityProfileSource_EmptyFactorScores_TreatedAsAbsent(t *testing.T) {
	t.Parallel()
	// 有 dimension_profile 标记但维度分为空（脏数据）→ 视为无人格画像，不得返回空 map
	fake := &fakeAssessmentForPersonality{items: []SurveyResultItem{
		{ResultID: 1, RiskLevel: "dimension_profile", SubmittedAt: 100, FactorScores: map[string]float64{}},
	}}
	src := NewPersonalityProfileSource(fake)

	got, err := src.LatestPersonalityProfile(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPersonalityProfileSource_ListError_Propagates(t *testing.T) {
	t.Parallel()
	fake := &fakeAssessmentForPersonality{err: errors.New("boom")}
	src := NewPersonalityProfileSource(fake)

	_, err := src.LatestPersonalityProfile(context.Background())
	require.Error(t, err)
}

func TestPersonalityProfileSource_NilClient_ReturnsNil(t *testing.T) {
	t.Parallel()
	src := NewPersonalityProfileSource(nil)
	got, err := src.LatestPersonalityProfile(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}
