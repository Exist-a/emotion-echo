// Package fusion — Stage 34 · PR-9 RED
//
// LateFuser 是 LLM-as-Fusion 失败 / 不可用时的兜底算法。
//
// 算法：late fusion 加权平均
//   - 输入：ModalitySnapshot{Text?, Face?, Voice?}，每个含 emotion/confidence/scores
//   - 输出：FusedEmotion
//     - primary_emotion: 取加权得分最高的 emotion
//     - sentiment_score: 按 confidence 加权平均各路 sentiment
//     - confidence: 各路 confidence 的加权平均
//     - modality_contrib: 各路权重（默认 text=0.4, voice=0.3, face=0.3；缺失则按比例重分配）
//     - fusion_method: "late_fusion_weighted"
//     - available_modalities: 实际用到的模态
//     - reasoning: 空（late_fusion 没有 LLM 解释）
//
// 设计原则：
//   - 单模态也能跑（只有 text 也算融合，权重=1）
//   - 零模态返回 error（不该调用，应由 Worker 保证至少 1 路）
package fusion

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeSnapshot 构造测试用 snapshot 帮手。
func makeSnapshot(text, face, voice *ModalityScore) ModalitySnapshot {
	return ModalitySnapshot{
		Text:  text,
		Face:  face,
		Voice: voice,
	}
}

// TestLateFuser_TextOnly_AllWeightOnText 只有文本：primary 来自 text，weight=1。
func TestLateFuser_TextOnly_AllWeightOnText(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3) // 默认权重

	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "sad", Confidence: 0.9, Sentiment: -0.5, Source: "text"},
		nil, nil,
	))
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, "sad", out.PrimaryEmotion)
	assert.InDelta(t, -0.5, out.SentimentScore, 0.001)
	assert.InDelta(t, 0.9, out.Confidence, 0.001)
	assert.Equal(t, "late_fusion_weighted", out.FusionMethod)
	require.Len(t, out.AvailableModalities, 1)
	assert.Equal(t, "text", out.AvailableModalities[0])
	// text 单独时 contribution 应为 1.0
	var textW float64
	require.NoError(t, jsonUnmarshalFloat(out.ModalityContrib, "text", &textW))
	assert.InDelta(t, 1.0, textW, 0.001)
}

// TestLateFuser_TextAndFace_TwoModalities 两路：权重按默认 0.4+0.3 重分配到 0.57/0.43。
func TestLateFuser_TextAndFace_TwoModalities(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3)

	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.8, Sentiment: 0.5, Source: "text"},
		&ModalityScore{Emotion: "neutral", Confidence: 0.7, Sentiment: 0.0, Source: "face"},
		nil,
	))
	require.NoError(t, err)
	require.NotNil(t, out)

	// 两路分数加权：text=0.5*0.8=0.4, face=0.0*0.7=0.0
	// happy 总分 = 0.4, neutral 总分 = 0.0 → primary = happy
	assert.Equal(t, "happy", out.PrimaryEmotion)
	// 权重归一化：text=0.4/(0.4+0.3)=0.5714, face=0.3/(0.4+0.3)=0.4286
	// sentiment = 0.5*0.5714 + 0.0*0.4286 ≈ 0.2857
	assert.InDelta(t, 0.2857, out.SentimentScore, 0.01)
	assert.Equal(t, "late_fusion_weighted", out.FusionMethod)
	require.Len(t, out.AvailableModalities, 2)
}

// TestLateFuser_AllThree_HappyFromText_FaceNeutral_VoiceAngry 三路：算术看加权。
func TestLateFuser_AllThree_HappyFromText_FaceNeutral_VoiceAngry(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3)

	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.8, Sentiment: 0.6, Source: "text"},
		&ModalityScore{Emotion: "neutral", Confidence: 0.7, Sentiment: 0.0, Source: "face"},
		&ModalityScore{Emotion: "angry", Confidence: 0.9, Sentiment: -0.7, Source: "voice"},
	))
	require.NoError(t, err)
	require.NotNil(t, out)

	// 各 emotion 加权得分：
	//   happy: 0.6*0.8=0.48
	//   neutral: 0.0*0.7=0.0
	//   angry: -0.7*0.9=-0.63
	// primary 取最高得分 → "happy"（angry 负分被排除）
	assert.Equal(t, "happy", out.PrimaryEmotion)
	assert.Equal(t, "late_fusion_weighted", out.FusionMethod)
	require.Len(t, out.AvailableModalities, 3)
}

// TestLateFuser_FaceOnly_WeightRedistributed 只有 face：contribution=1.0。
func TestLateFuser_FaceOnly_WeightRedistributed(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3)

	out, err := f.Fuse(context.Background(), makeSnapshot(
		nil,
		&ModalityScore{Emotion: "calm", Confidence: 0.95, Sentiment: 0.3, Source: "face"},
		nil,
	))
	require.NoError(t, err)
	assert.Equal(t, "calm", out.PrimaryEmotion)
	assert.InDelta(t, 0.3, out.SentimentScore, 0.001)
}

// TestLateFuser_NoModalities_ReturnsError 零模态：返回 error。
func TestLateFuser_NoModalities_ReturnsError(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3)

	_, err := f.Fuse(context.Background(), ModalitySnapshot{})
	require.Error(t, err)
}

// TestLateFuser_ModalityContribSumIsOne contribution 之和为 1。
func TestLateFuser_ModalityContribSumIsOne(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3)

	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.8, Sentiment: 0.5},
		&ModalityScore{Emotion: "neutral", Confidence: 0.7},
		&ModalityScore{Emotion: "angry", Confidence: 0.9},
	))
	require.NoError(t, err)

	// 解析 ModalityContrib JSON 求和
	var contribs map[string]float64
	require.NoError(t, jsonUnmarshalMap(out.ModalityContrib, &contribs))
	sum := contribs["text"] + contribs["face"] + contribs["voice"]
	assert.InDelta(t, 1.0, sum, 0.001, "modality_contrib must sum to 1.0")
}

// TestLateFuser_OutputIsFusedEmotion 输出是 *model.FusedEmotion（不是新类型）。
func TestLateFuser_OutputIsFusedEmotion(t *testing.T) {
	t.Parallel()
	f := NewWeightedLateFuser(0.4, 0.3, 0.3)

	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.8, Sentiment: 0.5},
		nil, nil,
	))
	require.NoError(t, err)
	// 编译期断言：*FusedEmotionOut 可以当 model.FusedEmotion 用
	var _ = out.MessageID
}
