package scoring

import (
	"testing"

	"emotion-echo-assessment-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================================
// PHQ-9 测试
// =====================================================

func TestPHQ9_NoneLevel_0to4(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 0, "q2": 1, "q3": 0, "q4": 1, "q5": 0, "q6": 0, "q7": 1, "q8": 0, "q9": 1}
	got, err := PHQ9Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 4.0, got.TotalScore, 0.001)
	assert.Equal(t, "none", got.RiskLevel)
}

func TestPHQ9_MildLevel_5to9(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	// 总分 = 5（边界值）
	ans := map[string]int{"q1": 1, "q2": 1, "q3": 1, "q4": 1, "q5": 0, "q6": 0, "q7": 1, "q8": 0, "q9": 0}
	got, err := PHQ9Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, got.TotalScore, 0.001)
	assert.Equal(t, "mild", got.RiskLevel)
}

func TestPHQ9_ModerateLevel_10to14(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 2, "q2": 2, "q3": 1, "q4": 1, "q5": 1, "q6": 1, "q7": 1, "q8": 1, "q9": 0}
	got, err := PHQ9Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, got.TotalScore, 0.001)
	assert.Equal(t, "moderate", got.RiskLevel)
}

func TestPHQ9_SevereLevel_15to19(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 2, "q2": 2, "q3": 2, "q4": 2, "q5": 2, "q6": 2, "q7": 2, "q8": 1, "q9": 0}
	got, err := PHQ9Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 15.0, got.TotalScore, 0.001)
	assert.Equal(t, "severe", got.RiskLevel)
}

func TestPHQ9_ExtremeLevel_20to27(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3, "q6": 3, "q7": 3, "q8": 3, "q9": 3}
	got, err := PHQ9Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 27.0, got.TotalScore, 0.001)
	assert.Equal(t, "extreme", got.RiskLevel)
}

func TestPHQ9_WrongAnswerCount_Error(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 1, "q2": 1, "q3": 1}
	_, err := PHQ9Scorer{}.Score(s, ans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "9 answers")
}

func TestPHQ9_OutOfRange_Error(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 4, "q2": 1, "q3": 1, "q4": 1, "q5": 1, "q6": 1, "q7": 1, "q8": 1, "q9": 1}
	_, err := PHQ9Scorer{}.Score(s, ans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0-3")
}

// =====================================================
// GAD-7 测试
// =====================================================

func TestGAD7_NoneLevel(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "GAD-7"}
	ans := map[string]int{"q1": 1, "q2": 1, "q3": 1, "q4": 0, "q5": 0, "q6": 0, "q7": 0}
	got, err := GAD7Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got.TotalScore, 0.001)
	assert.Equal(t, "none", got.RiskLevel)
}

func TestGAD7_SevereLevel_15to21(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "GAD-7"}
	ans := map[string]int{"q1": 3, "q2": 3, "q3": 3, "q4": 2, "q5": 2, "q6": 2, "q7": 2}
	got, err := GAD7Scorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 17.0, got.TotalScore, 0.001)
	assert.Equal(t, "severe", got.RiskLevel)
}

// =====================================================
// PSQI 测试
// =====================================================

func TestPSQI_NoneLevel_GoodSleep_0to5(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PSQI"}
	ans := map[string]int{"C1": 0, "C2": 1, "C3": 1, "C4": 0, "C5": 1, "C6": 0, "C7": 0}
	got, err := PSQIScorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got.TotalScore, 0.001)
	assert.Equal(t, "none", got.RiskLevel)
	assert.Len(t, got.Factors, 7)
}

func TestPSQI_MildLevel_6to10(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PSQI"}
	ans := map[string]int{"C1": 1, "C2": 1, "C3": 1, "C4": 1, "C5": 1, "C6": 0, "C7": 1}
	got, err := PSQIScorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 6.0, got.TotalScore, 0.001)
	assert.Equal(t, "mild", got.RiskLevel)
}

func TestPSQI_SevereLevel_16to21(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PSQI"}
	ans := map[string]int{"C1": 3, "C2": 2, "C3": 3, "C4": 2, "C5": 2, "C6": 2, "C7": 3}
	got, err := PSQIScorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 17.0, got.TotalScore, 0.001)
	assert.Equal(t, "severe", got.RiskLevel)
}

func TestPSQI_AcceptQKeysAsAlias(t *testing.T) {
	t.Parallel()
	// 兼容 q1..q7 写法（前端可能用 q）
	s := &model.Survey{Code: "PSQI"}
	ans := map[string]int{"q1": 0, "q2": 1, "q3": 0, "q4": 1, "q5": 0, "q6": 0, "q7": 0}
	got, err := PSQIScorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 2.0, got.TotalScore, 0.001)
	assert.Equal(t, "none", got.RiskLevel)
}

func TestPSQI_OutOfRange_Error(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PSQI"}
	ans := map[string]int{"C1": 5, "C2": 0, "C3": 0, "C4": 0, "C5": 0, "C6": 0, "C7": 0}
	_, err := PSQIScorer{}.Score(s, ans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0-3")
}

// =====================================================
// GenericScorer 兜底
// =====================================================

func TestGenericScorer_LowLevel(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "UNKNOWN"}
	ans := map[string]int{"q1": 1, "q2": 2}
	got, err := GenericScorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got.TotalScore, 0.001)
	// 3 / 20 = 0.15 < 0.4 → low
	assert.Equal(t, "low", got.RiskLevel)
}

func TestGenericScorer_HighLevel(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "UNKNOWN"}
	ans := map[string]int{"q1": 8, "q2": 9}
	got, err := GenericScorer{}.Score(s, ans)
	require.NoError(t, err)
	// 17 / 20 = 0.85 ≥ 0.7 → high
	assert.Equal(t, "high", got.RiskLevel)
}

// =====================================================
// GetScorer 路由
// =====================================================

func TestGetScorer_Dispatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		want string
	}{
		{"PHQ-9", "*scoring.PHQ9Scorer"},
		{"GAD-7", "*scoring.GAD7Scorer"},
		{"PSQI", "*scoring.PSQIScorer"},
		{"UNKNOWN", "*scoring.GenericScorer"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := GetScorer(tc.code)
			assert.NotNil(t, got)
		})
	}
}

// =====================================================
// 便捷 Score 函数
// =====================================================

func TestScore_AutoDispatchPHQ9(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "PHQ-9"}
	ans := map[string]int{"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3, "q6": 3, "q7": 3, "q8": 3, "q9": 3}
	got, err := Score(s, ans)
	require.NoError(t, err)
	assert.Equal(t, "extreme", got.RiskLevel)
	assert.InDelta(t, 27.0, got.TotalScore, 0.001)
}

func TestScore_GenericFallback(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "CUSTOM-100"}
	ans := map[string]int{"q1": 1}
	got, err := Score(s, ans)
	require.NoError(t, err)
	assert.Equal(t, "low", got.RiskLevel)
}

// =====================================================
// BIG5（人格五因素量表）测试
// =====================================================

// TestBigFiveScorer_AllNeutral_AllDimensionsMid 测试中立回答（全3分）
func TestBigFiveScorer_AllNeutral_AllDimensionsMid(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "BIG5"}
	ans := map[string]int{
		"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3,
		"q6": 3, "q7": 3, "q8": 3, "q9": 3, "q10": 3,
		"q11": 3, "q12": 3, "q13": 3, "q14": 3, "q15": 3,
		"q16": 3, "q17": 3, "q18": 3, "q19": 3, "q20": 3,
		"q21": 3, "q22": 3, "q23": 3, "q24": 3, "q25": 3,
		"q26": 3, "q27": 3, "q28": 3, "q29": 3, "q30": 3,
	}
	got, err := BigFiveScorer{}.Score(s, ans)
	require.NoError(t, err)
	// 每维度 6 题 × 3 分 = 18
	assert.InDelta(t, 18.0, got.Factors["openness"], 0.001)
	assert.InDelta(t, 18.0, got.Factors["conscientiousness"], 0.001)
	assert.InDelta(t, 18.0, got.Factors["extraversion"], 0.001)
	assert.InDelta(t, 18.0, got.Factors["agreeableness"], 0.001)
	assert.InDelta(t, 18.0, got.Factors["neuroticism"], 0.001)
	assert.Equal(t, "dimension_profile", got.RiskLevel)
	// 总分 = 5 × 18 = 90
	assert.InDelta(t, 90.0, got.TotalScore, 0.001)
}

// TestBigFiveScorer_HighOpenness 测试高开放性（q1/q6/q11/q16/q21/q26 全5分）
func TestBigFiveScorer_HighOpenness(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "BIG5"}
	ans := map[string]int{
		// 开放性全5
		"q1": 5, "q6": 5, "q11": 5, "q16": 5, "q21": 5, "q26": 5,
		// 其余中立
		"q2": 3, "q3": 3, "q4": 3, "q5": 3,
		"q7": 3, "q8": 3, "q9": 3, "q10": 3,
		"q12": 3, "q13": 3, "q14": 3, "q15": 3,
		"q17": 3, "q18": 3, "q19": 3, "q20": 3,
		"q22": 3, "q23": 3, "q24": 3, "q25": 3,
		"q27": 3, "q28": 3, "q29": 3, "q30": 3,
	}
	got, err := BigFiveScorer{}.Score(s, ans)
	require.NoError(t, err)
	assert.InDelta(t, 30.0, got.Factors["openness"], 0.001)
	assert.InDelta(t, 18.0, got.Factors["conscientiousness"], 0.001)
}

// TestBigFiveScorer_ReverseItems 测试反向计分题目
// q10（情绪稳定）、q15（不沮丧）、q20（不紧张）、q23（不喜欢独处）、q24（不冲突）、q25（不快乐）为反向题
func TestBigFiveScorer_ReverseItems(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "BIG5"}
	ans := map[string]int{
		// 正向题全3
		"q1": 3, "q2": 3, "q3": 3, "q4": 3, "q5": 3,
		"q6": 3, "q7": 3, "q8": 3, "q9": 3,
		"q11": 3, "q12": 3, "q13": 3, "q14": 3,
		"q16": 3, "q17": 3, "q18": 3, "q19": 3,
		"q21": 3, "q22": 3,
		"q26": 3, "q27": 3, "q28": 3, "q29": 3, "q30": 3,
		// 反向题：原始分5，反转后应为1
		"q10": 5, "q15": 5, "q20": 5, "q23": 5, "q24": 5, "q25": 5,
	}
	got, err := BigFiveScorer{}.Score(s, ans)
	require.NoError(t, err)
	// 神经质维度：q5=3, q10反转=1, q15反转=1, q20反转=1, q25反转=1, q30=3 → 10
	assert.InDelta(t, 10.0, got.Factors["neuroticism"], 0.001)
}

// TestBigFiveScorer_WrongAnswerCount_Error 测试答案数错误
func TestBigFiveScorer_WrongAnswerCount_Error(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "BIG5"}
	ans := map[string]int{"q1": 3, "q2": 3}
	_, err := BigFiveScorer{}.Score(s, ans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30 answers")
}

// TestBigFiveScorer_OutOfRange_Error 测试超范围分数
func TestBigFiveScorer_OutOfRange_Error(t *testing.T) {
	t.Parallel()
	s := &model.Survey{Code: "BIG5"}
	ans := map[string]int{
		"q1": 6, // 超范围
		"q2": 3, "q3": 3, "q4": 3, "q5": 3,
		"q6": 3, "q7": 3, "q8": 3, "q9": 3, "q10": 3,
		"q11": 3, "q12": 3, "q13": 3, "q14": 3, "q15": 3,
		"q16": 3, "q17": 3, "q18": 3, "q19": 3, "q20": 3,
		"q21": 3, "q22": 3, "q23": 3, "q24": 3, "q25": 3,
		"q26": 3, "q27": 3, "q28": 3, "q29": 3, "q30": 3,
	}
	_, err := BigFiveScorer{}.Score(s, ans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1-5")
}

// TestBigFiveScorer_GetScorer_Dispatch 测试调度器路由
func TestGetScorer_BIG5_Dispatches(t *testing.T) {
	t.Parallel()
	got := GetScorer("BIG5")
	assert.NotNil(t, got)
	_, ok := got.(BigFiveScorer)
	assert.True(t, ok, "expected BigFiveScorer")
}