// Package downstream — personality.go
//
// E2E-14：人格画像来源（BFF → assessment-svc）。
//
// 用途：/api/v1/ai/stream 组装 system prompt 时注入用户人格画像，使 AI 回复
// 贴合用户特质（D-02 决议：人格量表驱动 AI 提示词定制）。
//
// 取数：调 assessment-svc 的"我的结果"列表，挑出最新一条人格量表结果。
// 判据是 riskLevel == "dimension_profile" —— BigFiveScorer 写入的固定标记
// （人格量表无"风险"概念，用该值区别于症状量表的 none/mild/moderate/...）。
//
// 缺画像（未测评 / 脏数据 / 上游报错）不是错误路径：调用方回落基础人设 prompt。
package downstream

import (
	"context"
)

// personalityScanLimit 扫描"我的结果"的条数上限。
// 人格量表通常只有 1~2 条，20 条足够覆盖"最近测评过人格量表、之后又做了若干症状量表"的情形。
const personalityScanLimit = 20

// personalityRiskLevelMarker 人格量表结果的 riskLevel 标记值（见 scoring.BigFiveScorer）
const personalityRiskLevelMarker = "dimension_profile"

// PersonalityProfileSource 从 assessment-svc 取当前用户最新人格画像
type PersonalityProfileSource struct {
	client AssessmentClient
}

// NewPersonalityProfileSource 构造
func NewPersonalityProfileSource(client AssessmentClient) *PersonalityProfileSource {
	return &PersonalityProfileSource{client: client}
}

// LatestPersonalityProfile 返回当前用户最新人格量表结果的维度分。
//
// 返回值：
//   - (dims, nil)  找到人格结果且维度分非空
//   - (nil, nil)   没有（可用）的人格结果 —— 正常情形，调用方回落基础 prompt
//   - (nil, err)   上游查询失败 —— 调用方同样回落，只记日志
//
// 用户身份来自 ctx（x-user-id 由 session 中间件注入），与其它 downstream 调用一致。
func (s *PersonalityProfileSource) LatestPersonalityProfile(ctx context.Context) (map[string]float64, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}

	items, _, err := s.client.ListResults(ctx, personalityScanLimit)
	if err != nil {
		return nil, err
	}

	// 不依赖上游排序：显式取 submittedAt 最大的那条，多一个来源变更也不会取错。
	var (
		latest   map[string]float64
		latestAt int64
		foundAny bool
	)
	for _, it := range items {
		if it.RiskLevel != personalityRiskLevelMarker {
			continue
		}
		if len(it.FactorScores) == 0 {
			continue // 有标记无维度分（脏数据）→ 当作没有
		}
		if foundAny && it.SubmittedAt <= latestAt {
			continue
		}
		latest = it.FactorScores
		latestAt = it.SubmittedAt
		foundAny = true
	}
	return latest, nil
}
