// Package fusion — Stage 35 · PR-2 GREEN
//
// validateLLMOutput 校验 LLM 反序列化后的 llmFusedOutput 结构是否合法。
//
// 校验三条（任一失败 → 返回 error，让 Worker 走 late_fuser 兜底）：
//   1. PrimaryEmotion ∈ 白名单（happy/sad/angry/neutral/calm/anxious/surprised/disgusted/fearful）
//   2. SentimentScore ∈ [-1, 1]（闭区间）
//   3. ModalityContrib 非空、各 value ∈ [0, 1]、总和 ∈ [0.99, 1.01]（容忍浮点）
//
// 设计理由（ADR-15 §B）：
//   - 白名单比正则稳：emotion 标签体系是封闭集合
//   - 浮点和用 ε 容差：避免 0.333+0.333+0.334 这种 LLM 输出导致误判
//   - 不引入 go-playground/validator：避免 reflect 运行时成本，且 emotion 白名单动态配置反而更复杂
package fusion

import (
	"fmt"
	"math"
)

// allowedEmotions LLM 输出 emotion 白名单。
//
// 与 `model` 包里的 emotion 标签体系对齐；新增标签时同步更新。
var allowedEmotions = map[string]struct{}{
	"happy":     {},
	"sad":       {},
	"angry":     {},
	"neutral":   {},
	"calm":      {},
	"anxious":   {},
	"surprised": {},
	"disgusted": {},
	"fearful":   {},
}

// modalityContribSumTolerance modality_contrib 总和容差（浮点和）。
const modalityContribSumTolerance = 0.01

// validateLLMOutput 校验 LLM 输出合法性。
//
// 返回 nil 表示合法；否则返回带字段名的 error，便于日志定位。
func validateLLMOutput(o llmFusedOutput) error {
	// 1. emotion 白名单
	if _, ok := allowedEmotions[o.PrimaryEmotion]; !ok {
		return fmt.Errorf("invalid primary_emotion=%q (allowed: happy/sad/angry/neutral/calm/anxious/surprised/disgusted/fearful)", o.PrimaryEmotion)
	}

	// 2. sentiment ∈ [-1, 1]
	if o.SentimentScore < -1.0 || o.SentimentScore > 1.0 || math.IsNaN(o.SentimentScore) {
		return fmt.Errorf("invalid sentiment_score=%v (must be in [-1, 1])", o.SentimentScore)
	}

	// 3. modality_contrib 非空
	if len(o.ModalityContrib) == 0 {
		return fmt.Errorf("invalid modality_contrib: empty")
	}

	// 3a. 各 value ∈ [0, 1]
	sum := 0.0
	for k, v := range o.ModalityContrib {
		if v < 0.0 || v > 1.0 || math.IsNaN(v) {
			return fmt.Errorf("invalid modality_contrib[%q]=%v (must be in [0, 1])", k, v)
		}
		sum += v
	}

	// 3b. 总和 ∈ [1-ε, 1+ε]
	if math.Abs(sum-1.0) > modalityContribSumTolerance {
		return fmt.Errorf("invalid modality_contrib sum=%v (must be 1.0 ± %v)", sum, modalityContribSumTolerance)
	}

	return nil
}