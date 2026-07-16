package assessment

import (
	"math"
	"strings"

	"emotion-echo-gin/internal/workflow/graph"
)

// RiskKeywords 风险关键词列表
var RiskKeywords = []string{
	"自杀", "跳楼", "割腕", "上吊", "想死", "不想活了",
	"结束生命", "了结", "解脱", "没意义", "活着没意思",
	"绝望", "撑不下去", "撑不住", " hopeless", "想消失",
	"自残", "伤害自己", "身体疼痛", "痛苦", "受不了",
	"结束痛苦", "了结自己", "不想坚持", "放弃生命",
}

// CalculateDimensionScores 计算五维评分（已移除睡眠维度）
func CalculateDimensionScores(
	emotionStability, positiveRatio, negativeIntensity, regulationAbility int,
	sdsScore, sasScore int,
	msgFrequency, nightActivityRatio, avgSessionDepth float64,
) *DimensionScores {
	// 情绪维度 (0-100)
	emotionScore := int(float64(emotionStability)*0.4 + float64(positiveRatio)*0.3 + float64(regulationAbility)*0.3)

	// 抑郁维度 (SDS 标准分 25-100，转换为 0-100，越高越健康)
	depressionScore := int(math.Max(0, 100-float64(sdsScore-25)*1.33))

	// 焦虑维度 (SAS 标准分 25-100，转换为 0-100)
	anxietyScore := int(math.Max(0, 100-float64(sasScore-25)*1.33))

	// 压力维度 (基于消息频率、倾诉深度)
	stressScore := calculateStressScore(msgFrequency, avgSessionDepth)

	// 社交维度 (基于活跃度)
	socialScore := calculateSocialScore(msgFrequency, nightActivityRatio)

	return &DimensionScores{
		Emotion:    clamp(emotionScore, 0, 100),
		Depression: clamp(depressionScore, 0, 100),
		Anxiety:    clamp(anxietyScore, 0, 100),
		Stress:     clamp(stressScore, 0, 100),
		Social:     clamp(socialScore, 0, 100),
	}
}

// CalculateOverallScore 计算综合评分（五维，已移除睡眠）
func CalculateOverallScore(scores *DimensionScores, textRiskSeverity string) (int, string, []string) {
	// 基础综合分（加权平均）
	overall := float64(scores.Emotion)*0.28 +
		float64(scores.Depression)*0.22 +
		float64(scores.Anxiety)*0.22 +
		float64(scores.Stress)*0.16 +
		float64(scores.Social)*0.12

	// 风险因子
	riskFactors := []string{}

	if scores.Depression < 40 {
		overall -= 15
		riskFactors = append(riskFactors, "抑郁风险较高")
	}
	if scores.Anxiety < 40 {
		overall -= 10
		riskFactors = append(riskFactors, "焦虑风险较高")
	}
	if scores.Emotion < 40 {
		overall -= 8
		riskFactors = append(riskFactors, "情绪波动较大")
	}
	if textRiskSeverity == "high" {
		overall -= 25
		riskFactors = append(riskFactors, "检测到高风险文本")
	} else if textRiskSeverity == "moderate" {
		overall -= 12
		riskFactors = append(riskFactors, "检测到潜在风险文本")
	}

	overallInt := clamp(int(overall), 0, 100)

	// 确定风险等级
	var level string
	switch {
	case overallInt >= 90:
		level = "low"
	case overallInt >= 70:
		level = "medium"
	case overallInt >= 40:
		level = "high"
	default:
		level = "critical"
	}

	return overallInt, level, riskFactors
}

// DetectTextRisk 检测文本风险
func DetectTextRisk(content string) (bool, int, string, []string) {
	content = strings.ToLower(content)
	riskContexts := []string{}
	count := 0

	for _, keyword := range RiskKeywords {
		if strings.Contains(content, strings.ToLower(keyword)) {
			count++
			// 提取上下文（关键词前后20个字符）
			idx := strings.Index(content, strings.ToLower(keyword))
			if idx != -1 {
				start := int(math.Max(0, float64(idx-20)))
				end := int(math.Min(float64(len(content)), float64(idx+len(keyword)+20)))
				context := content[start:end]
				riskContexts = append(riskContexts, context)
			}
		}
	}

	if count == 0 {
		return false, 0, "none", nil
	}

	// 判断严重程度
	severity := "low"
	if count >= 5 {
		severity = "high"
	} else if count >= 2 {
		severity = "moderate"
	}

	return true, count, severity, riskContexts
}

// DimensionScores 五维评分（已移除睡眠维度）
type DimensionScores struct {
	Emotion    int
	Depression int
	Anxiety    int
	Stress     int
	Social     int
}

// calculateStressScore 计算压力分数
func calculateStressScore(msgFrequency, avgSessionDepth float64) int {
	// 消息频率越高、会话深度越大，压力可能越大
	score := 100 - int(msgFrequency*2) - int(avgSessionDepth*3)
	return clamp(score, 0, 100)
}

// calculateSocialScore 计算社交分数
func calculateSocialScore(msgFrequency, nightActivityRatio float64) int {
	// 消息频率高 = 社交活跃
	// 夜间活跃度低 = 社交健康
	score := int(msgFrequency*3) - int(nightActivityRatio*50)
	return clamp(score, 0, 100)
}

// clamp 限制数值范围
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// RiskLevelCondition 风险等级条件
type RiskLevelCondition struct {
	Level string
}

func (c *RiskLevelCondition) Evaluate(state graph.State) bool {
	return state.GetString("risk_level") == c.Level
}
