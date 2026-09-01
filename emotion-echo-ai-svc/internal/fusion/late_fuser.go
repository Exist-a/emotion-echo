// Package fusion — Stage 34 · PR-10 GREEN
//
// WeightedLateFuser 是 LLM-as-Fusion 失败时的兜底算法。
//
// 算法：late fusion 加权投票
//   1. 收集所有可用模态的 (Emotion, Confidence, Sentiment)
//   2. 按用户配置的默认权重（text=0.4, voice=0.3, face=0.3）重新归一化
//      — 缺失模态的权重按比例分配给剩余模态
//   3. 对每个 (modality, emotion) 计算 score = Sentiment × Confidence × Weight
//   4. 取得分最高的 emotion 作为 primary（如果最高分是负数，则回退到 confidence 最高的模态）
//   5. sentiment_score = Σ (Sentiment_i × Weight_i) / Σ Weight_i
//   6. confidence = Σ (Confidence_i × Weight_i) / Σ Weight_i
//   7. modality_contrib: 各模态归一化权重（JSON 字符串）
//
// 输出是 *model.FusedEmotion（与 Worker 写库契约一致）。
package fusion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"emotion-echo-ai-svc/internal/model"
)

// Fuser 融合器接口（LateFuser 和 LLMFuser 都实现）。
type Fuser interface {
	Fuse(ctx context.Context, s ModalitySnapshot) (*model.FusedEmotion, error)
}

// WeightedLateFuser 加权 late fusion 实现。
type WeightedLateFuser struct {
	wText  float64
	wVoice float64
	wFace  float64
}

// NewWeightedLateFuser 构造器（默认 0.4 / 0.3 / 0.3，建议总和=1）。
func NewWeightedLateFuser(wText, wVoice, wFace float64) *WeightedLateFuser {
	return &WeightedLateFuser{wText: wText, wVoice: wVoice, wFace: wFace}
}

// Fuse 是 Fuser 接口实现。
func (f *WeightedLateFuser) Fuse(ctx context.Context, s ModalitySnapshot) (*model.FusedEmotion, error) {
	if s.IsEmpty() {
		return nil, errors.New("fusion: no modalities available")
	}

	// 1. 收集有效模态
	type slot struct {
		name     string
		score    *ModalityScore
		weight   float64
	}
	slots := make([]slot, 0, 3)
	if s.Text != nil {
		slots = append(slots, slot{"text", s.Text, f.wText})
	}
	if s.Voice != nil {
		slots = append(slots, slot{"voice", s.Voice, f.wVoice})
	}
	if s.Face != nil {
		slots = append(slots, slot{"face", s.Face, f.wFace})
	}

	// 2. 归一化权重（缺失模态权重重新分配）
	totalW := 0.0
	for _, sl := range slots {
		totalW += sl.weight
	}
	if totalW <= 0 {
		// 配置错误 → 平均分配
		eq := 1.0 / float64(len(slots))
		for i := range slots {
			slots[i].weight = eq
		}
		totalW = 1.0
	} else {
		for i := range slots {
			slots[i].weight = slots[i].weight / totalW
		}
	}

	// 3. 聚合 emotion 分数
	type emoScore struct {
		label string
		score float64
	}
	emoScores := map[string]float64{}
	for _, sl := range slots {
		// 单路对某 emotion 的得分 = Sentiment × Confidence × Weight
		emoScores[sl.score.Emotion] += sl.score.Sentiment * sl.score.Confidence * sl.weight
	}

	// 4. 选 primary：最高正分 emotion；若全 ≤0，取 confidence 最高的模态的 emotion
	primary, maxScore := "", -1e9
	for label, sc := range emoScores {
		if sc > maxScore {
			maxScore = sc
			primary = label
		}
	}
	if maxScore <= 0 {
		// 全是 ≤0（极端情况）：取 confidence 最高的模态
		best := slots[0]
		for _, sl := range slots[1:] {
			if sl.score.Confidence > best.score.Confidence {
				best = sl
			}
		}
		primary = best.score.Emotion
	}

	// 5. 加权平均 sentiment + confidence
	sentiment := 0.0
	confidence := 0.0
	for _, sl := range slots {
		sentiment += sl.score.Sentiment * sl.weight
		confidence += sl.score.Confidence * sl.weight
	}

	// 6. 构造 modality_contrib JSON
	contrib := make(map[string]float64, len(slots))
	for _, sl := range slots {
		contrib[sl.name] = sl.weight
	}
	contribJSON, err := json.Marshal(contrib)
	if err != nil {
		return nil, fmt.Errorf("marshal modality_contrib: %w", err)
	}

	// 7. available_modalities
	modalities := s.AvailableModalities()

	return &model.FusedEmotion{
		PrimaryEmotion:      primary,
		SentimentScore:      sentiment,
		Confidence:          confidence,
		ModalityContrib:     string(contribJSON),
		Reasoning:           "", // late_fusion 无 LLM 解释
		FusionMethod:        "late_fusion_weighted",
		AvailableModalities: model.AvailableModalitiesFromSlice(modalities),
	}, nil
}
