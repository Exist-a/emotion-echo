// Package fusion — Stage 35 · PR-2 RED
//
// validateLLMOutput 校验 LLM 反序列化后的 llmFusedOutput 结构是否合法。
//
// 校验三条：
//   1. PrimaryEmotion 在白名单内（happy/sad/angry/neutral/calm/anxious/...）
//   2. SentimentScore ∈ [-1, 1]
//   3. ModalityContrib 非空、各 value ∈ [0, 1]、总和 ∈ [0.99, 1.01]
package fusion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateLLMOutput_Valid 完全合法 → nil。
func TestValidateLLMOutput_Valid(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happy",
		SentimentScore:  0.5,
		ModalityContrib: map[string]float64{"text": 0.6, "voice": 0.4},
		Reasoning:       "ok",
	}
	require.NoError(t, validateLLMOutput(out))
}

// TestValidateLLMOutput_EmptyEmotion 缺 emotion → error。
func TestValidateLLMOutput_EmptyEmotion(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "",
		SentimentScore:  0.0,
		ModalityContrib: map[string]float64{"text": 1.0},
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary_emotion")
}

// TestValidateLLMOutput_UnknownEmotion 非法 emotion → error。
func TestValidateLLMOutput_UnknownEmotion(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happyness", // 拼错
		SentimentScore:  0.5,
		ModalityContrib: map[string]float64{"text": 1.0},
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary_emotion")
}

// TestValidateLLMOutput_SentimentOutOfRange sentiment > 1 → error。
func TestValidateLLMOutput_SentimentOutOfRange(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happy",
		SentimentScore:  1.5,
		ModalityContrib: map[string]float64{"text": 1.0},
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sentiment_score")
}

// TestValidateLLMOutput_SentimentNegativeOutOfRange sentiment < -1 → error。
func TestValidateLLMOutput_SentimentNegativeOutOfRange(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "sad",
		SentimentScore:  -1.5,
		ModalityContrib: map[string]float64{"text": 1.0},
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sentiment_score")
}

// TestValidateLLMOutput_SentimentBoundaryOK sentiment = ±1 → ok。
func TestValidateLLMOutput_SentimentBoundaryOK(t *testing.T) {
	t.Parallel()
	for _, s := range []float64{-1.0, 0.0, 1.0} {
		out := llmFusedOutput{
			PrimaryEmotion:  "neutral",
			SentimentScore:  s,
			ModalityContrib: map[string]float64{"text": 1.0},
		}
		assert.NoError(t, validateLLMOutput(out), "sentiment=%v should be valid", s)
	}
}

// TestValidateLLMOutput_EmptyModalityContrib contrib 为空 → error。
func TestValidateLLMOutput_EmptyModalityContrib(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happy",
		SentimentScore:  0.0,
		ModalityContrib: map[string]float64{},
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modality_contrib")
}

// TestValidateLLMOutput_ModalityContribSumNotOne 总和 ≠ 1 → error。
func TestValidateLLMOutput_ModalityContribSumNotOne(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happy",
		SentimentScore:  0.0,
		ModalityContrib: map[string]float64{"text": 0.5, "voice": 0.3}, // 总和 0.8
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modality_contrib")
}

// TestValidateLLMOutput_ModalityContribValueOutOfRange value > 1 → error。
func TestValidateLLMOutput_ModalityContribValueOutOfRange(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happy",
		SentimentScore:  0.0,
		ModalityContrib: map[string]float64{"text": 1.5}, // value 越界
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modality_contrib")
}

// TestValidateLLMOutput_ModalityContribNegativeValue value < 0 → error。
func TestValidateLLMOutput_ModalityContribNegativeValue(t *testing.T) {
	t.Parallel()
	out := llmFusedOutput{
		PrimaryEmotion:  "happy",
		SentimentScore:  0.0,
		ModalityContrib: map[string]float64{"text": 1.2, "voice": -0.2}, // 负值 + 总和=1
	}
	err := validateLLMOutput(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modality_contrib")
}

// TestValidateLLMOutput_ModalityContribSumTolerance 总和 0.999 / 1.001 → ok（浮点容差）。
func TestValidateLLMOutput_ModalityContribSumTolerance(t *testing.T) {
	t.Parallel()
	cases := []map[string]float64{
		{"text": 0.333, "voice": 0.333, "face": 0.334}, // ≈ 1.0
		{"text": 0.5, "voice": 0.5},                    // = 1.0
	}
	for _, c := range cases {
		out := llmFusedOutput{
			PrimaryEmotion:  "neutral",
			SentimentScore:  0.0,
			ModalityContrib: c,
		}
		assert.NoError(t, validateLLMOutput(out), "contrib=%v should be valid", c)
	}
}