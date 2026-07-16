// Package scoring 提供量表真实计分规则
//
// 设计：
//   - 按 survey.Code 选 scorer（PHQ-9 / GAD-7 / PSQI / Generic）
//   - Scorer 返回 total_score + risk_level + factors（可选）
//   - 失败兜底为 GenericScorer（保留兼容）
//
// 后续可以扩展：
//   - 自定义量表 SQL 配置
//   - 复杂计分规则（factor scores）
//   - 严重程度描述（advice / suggested action）
package scoring

import (
	"fmt"

	"emotion-echo-assessment-svc/internal/model"
)

// Result 计分结果
type Result struct {
	TotalScore float64            // 总分
	RiskLevel  string             // 风险等级：none / mild / moderate / severe / extreme
	Factors    map[string]float64 // 分项分数（如 PSQI 7 个 component）
}

// Scorer 计分器接口
type Scorer interface {
	Score(survey *model.Survey, answers map[string]int) (*Result, error)
}

// =====================================================
// PHQ-9（Patient Health Questionnaire-9 抑郁）
// =====================================================
//
// 9 题，每题 0-3（0=完全没有, 1=几天, 2=一半以上天数, 3=几乎每天）
// 总分范围 0-27
//
// 等级：
//   0-4  : none      (无/最小)
//   5-9  : mild      (轻度)
//   10-14: moderate  (中度)
//   15-19: severe    (重度)
//   20-27: extreme   (极重度)
type PHQ9Scorer struct{}

func (PHQ9Scorer) Score(s *model.Survey, answers map[string]int) (*Result, error) {
	if len(answers) != 9 {
		return nil, fmt.Errorf("PHQ-9 requires exactly 9 answers, got %d", len(answers))
	}
	var total float64
	for i := 1; i <= 9; i++ {
		key := fmt.Sprintf("q%d", i)
		v, ok := answers[key]
		if !ok {
			return nil, fmt.Errorf("PHQ-9 missing answer for %s", key)
		}
		if v < 0 || v > 3 {
			return nil, fmt.Errorf("PHQ-9 answer %s must be 0-3, got %d", key, v)
		}
		total += float64(v)
	}

	level := "none"
	switch {
	case total >= 20:
		level = "extreme"
	case total >= 15:
		level = "severe"
	case total >= 10:
		level = "moderate"
	case total >= 5:
		level = "mild"
	}

	return &Result{
		TotalScore: total,
		RiskLevel:  level,
		Factors: map[string]float64{
			"q1": float64(answers["q1"]),
			"q2": float64(answers["q2"]),
			"q3": float64(answers["q3"]),
			"q4": float64(answers["q4"]),
			"q5": float64(answers["q5"]),
			"q6": float64(answers["q6"]),
			"q7": float64(answers["q7"]),
			"q8": float64(answers["q8"]),
			"q9": float64(answers["q9"]),
		},
	}, nil
}

// =====================================================
// GAD-7（Generalized Anxiety Disorder-7 焦虑）
// =====================================================
//
// 7 题，每题 0-3
// 总分范围 0-21
//
// 等级：
//   0-4  : none     (无)
//   5-9  : mild     (轻度)
//   10-14: moderate (中度)
//   15-21: severe   (重度)
type GAD7Scorer struct{}

func (GAD7Scorer) Score(s *model.Survey, answers map[string]int) (*Result, error) {
	if len(answers) != 7 {
		return nil, fmt.Errorf("GAD-7 requires exactly 7 answers, got %d", len(answers))
	}
	var total float64
	for i := 1; i <= 7; i++ {
		key := fmt.Sprintf("q%d", i)
		v, ok := answers[key]
		if !ok {
			return nil, fmt.Errorf("GAD-7 missing answer for %s", key)
		}
		if v < 0 || v > 3 {
			return nil, fmt.Errorf("GAD-7 answer %s must be 0-3, got %d", key, v)
		}
		total += float64(v)
	}

	level := "none"
	switch {
	case total >= 15:
		level = "severe"
	case total >= 10:
		level = "moderate"
	case total >= 5:
		level = "mild"
	}

	return &Result{
		TotalScore: total,
		RiskLevel:  level,
		Factors: map[string]float64{
			"q1": float64(answers["q1"]),
			"q2": float64(answers["q2"]),
			"q3": float64(answers["q3"]),
			"q4": float64(answers["q4"]),
			"q5": float64(answers["q5"]),
			"q6": float64(answers["q6"]),
			"q7": float64(answers["q7"]),
		},
	}, nil
}

// =====================================================
// PSQI（Pittsburgh Sleep Quality Index 睡眠质量）
// =====================================================
//
// 7 个 component（C1-C7），每个 0-3
//
// C1 主观睡眠质量 (subjective sleep quality)
// C2 入睡时间 (sleep latency)
// C3 睡眠时间 (sleep duration)
// C4 睡眠效率 (habitual sleep efficiency)
// C5 睡眠障碍 (sleep disturbances)
// C6 催眠药物 (use of sleeping medication)
// C7 日间功能障碍 (daytime dysfunction)
//
// 总分 0-21
//
// 等级：
//   0-5  : none     (好)
//   6-10 : mild     (中)
//   11-15: moderate (差)
//   16-21: severe   (很差)
//
// 输入：answers 是 component 字典 {C1: 0-3, ..., C7: 0-3}
// 也可以接受 {q1: ..., q7: ...}（兼容写法）
type PSQIScorer struct{}

func (PSQIScorer) Score(s *model.Survey, answers map[string]int) (*Result, error) {
	// 兼容两种 key：C1/C2/... 或 q1/q2/...
	get := func(idx int) (int, bool) {
		ck := fmt.Sprintf("C%d", idx)
		if v, ok := answers[ck]; ok {
			return v, true
		}
		qk := fmt.Sprintf("q%d", idx)
		if v, ok := answers[qk]; ok {
			return v, true
		}
		return 0, false
	}

	if len(answers) != 7 {
		return nil, fmt.Errorf("PSQI requires exactly 7 component scores, got %d", len(answers))
	}

	factors := make(map[string]float64, 7)
	var total float64
	for i := 1; i <= 7; i++ {
		v, ok := get(i)
		if !ok {
			return nil, fmt.Errorf("PSQI missing component C%d/q%d", i, i)
		}
		if v < 0 || v > 3 {
			return nil, fmt.Errorf("PSQI component C%d must be 0-3, got %d", i, v)
		}
		factors[fmt.Sprintf("C%d", i)] = float64(v)
		total += float64(v)
	}

	level := "none"
	switch {
	case total >= 16:
		level = "severe"
	case total >= 11:
		level = "moderate"
	case total >= 6:
		level = "mild"
	}

	return &Result{
		TotalScore: total,
		RiskLevel:  level,
		Factors:    factors,
	}, nil
}

// =====================================================
// GenericScorer（兜底：未知量表用）
// =====================================================
//
// 通用计分规则：
//   total_score = sum(answers)
//   answered = count(answers)
//   level: ≥0.7 → high; ≥0.4 → medium; <0.4 → low
type GenericScorer struct{}

func (GenericScorer) Score(s *model.Survey, answers map[string]int) (*Result, error) {
	var total float64
	factors := make(map[string]float64, len(answers))
	for k, v := range answers {
		if v < 0 || v > 10 {
			return nil, fmt.Errorf("answer %s must be 0-10, got %d", k, v)
		}
		total += float64(v)
		factors[k] = float64(v)
	}

	level := "low"
	maxScore := float64(len(answers)) * 10
	if maxScore > 0 {
		ratio := total / maxScore
		switch {
		case ratio >= 0.7:
			level = "high"
		case ratio >= 0.4:
			level = "medium"
		}
	}

	return &Result{
		TotalScore: total,
		RiskLevel:  level,
		Factors:    factors,
	}, nil
}

// =====================================================
// 调度器（按 survey.Code 选 scorer）
// =====================================================

// GetScorer 按 survey.Code 返回匹配的 scorer，未知用 GenericScorer
func GetScorer(code string) Scorer {
	switch code {
	case "PHQ-9":
		return PHQ9Scorer{}
	case "GAD-7":
		return GAD7Scorer{}
	case "PSQI":
		return PSQIScorer{}
	default:
		return GenericScorer{}
	}
}

// Score 是便捷函数：自动选 scorer
func Score(survey *model.Survey, answers map[string]int) (*Result, error) {
	return GetScorer(survey.Code).Score(survey, answers)
}