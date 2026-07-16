package service

import "fmt"

// GenerateDailySummary 生成日报摘要
func GenerateDailySummary(dist []EmotionDistribution, convCount, msgCount int) string {
	dominantEmotion := "中性"
	maxValue := 0
	for _, d := range dist {
		if d.Value > maxValue {
			maxValue = d.Value
			dominantEmotion = TranslateEmotionLabel(d.Name)
		}
	}

	return fmt.Sprintf("今日情绪整体偏%s，共进行了%d次对话，发送了%d条消息。建议保持积极心态，适当放松心情。",
		dominantEmotion, convCount, msgCount)
}

// GenerateTrendSummary 生成趋势摘要
func GenerateTrendSummary(reportType string, dist []EmotionDistribution, convCount, msgCount int) string {
	dominantEmotion := "中性"
	maxValue := 0
	for _, d := range dist {
		if d.Value > maxValue {
			maxValue = d.Value
			dominantEmotion = TranslateEmotionLabel(d.Name)
		}
	}

	period := "本周"
	switch reportType {
	case "monthly":
		period = "本月"
	case "yearly":
		period = "今年"
	}

	return fmt.Sprintf("%s情绪整体偏%s，共进行了%d次对话，发送了%d条消息。建议保持积极心态，适当放松心情。",
		period, dominantEmotion, convCount, msgCount)
}
