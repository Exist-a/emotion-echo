package service

import (
	"emotion-echo-gin/internal/models"
	"time"
)

// AggregateEmotionTrendByDay 聚合情绪趋势数据（按天）
func AggregateEmotionTrendByDay(analyses []*models.EmotionAnalysis, days int, startDate time.Time) []SeriesData {
	happyData := make([]int, days)
	sadData := make([]int, days)
	angryData := make([]int, days)
	anxiousData := make([]int, days)
	neutralData := make([]int, days)

	happyCount := make([]int, days)
	sadCount := make([]int, days)
	angryCount := make([]int, days)
	anxiousCount := make([]int, days)
	neutralCount := make([]int, days)

	for _, a := range analyses {
		dayIndex := int(a.AnalyzedAt.Sub(startDate).Hours() / 24)
		if dayIndex >= 0 && dayIndex < days {
			if scores := a.EmotionScores; scores != nil {
				if v, ok := scores["happy"]; ok {
					happyData[dayIndex] += int(v * 100)
					happyCount[dayIndex]++
				}
				if v, ok := scores["sad"]; ok {
					sadData[dayIndex] += int(v * 100)
					sadCount[dayIndex]++
				}
				if v, ok := scores["angry"]; ok {
					angryData[dayIndex] += int(v * 100)
					angryCount[dayIndex]++
				}
				if v, ok := scores["anxious"]; ok {
					anxiousData[dayIndex] += int(v * 100)
					anxiousCount[dayIndex]++
				}
				if v, ok := scores["neutral"]; ok {
					neutralData[dayIndex] += int(v * 100)
					neutralCount[dayIndex]++
				}
			}
		}
	}

	for i := 0; i < days; i++ {
		if happyCount[i] > 0 {
			happyData[i] = happyData[i] / happyCount[i]
		}
		if sadCount[i] > 0 {
			sadData[i] = sadData[i] / sadCount[i]
		}
		if angryCount[i] > 0 {
			angryData[i] = angryData[i] / angryCount[i]
		}
		if anxiousCount[i] > 0 {
			anxiousData[i] = anxiousData[i] / anxiousCount[i]
		}
		if neutralCount[i] > 0 {
			neutralData[i] = neutralData[i] / neutralCount[i]
		}
	}

	return []SeriesData{
		{Name: "开心指数", Data: happyData},
		{Name: "悲伤指数", Data: sadData},
		{Name: "愤怒指数", Data: angryData},
		{Name: "焦虑指数", Data: anxiousData},
		{Name: "中性指数", Data: neutralData},
	}
}

// AggregateEmotionTrendByMonth 聚合情绪趋势数据（按月）
func AggregateEmotionTrendByMonth(analyses []*models.EmotionAnalysis, months int, startDate time.Time) []SeriesData {
	happySum := make([]float64, months)
	sadSum := make([]float64, months)
	angrySum := make([]float64, months)
	anxiousSum := make([]float64, months)
	neutralSum := make([]float64, months)
	happyCount := make([]int, months)
	sadCount := make([]int, months)
	angryCount := make([]int, months)
	anxiousCount := make([]int, months)
	neutralCount := make([]int, months)

	for _, a := range analyses {
		yearDiff := a.AnalyzedAt.Year() - startDate.Year()
		monthDiff := int(a.AnalyzedAt.Month()) - int(startDate.Month())
		monthIndex := yearDiff*12 + monthDiff

		if monthIndex >= 0 && monthIndex < months {
			if scores := a.EmotionScores; scores != nil {
				if v, ok := scores["happy"]; ok {
					happySum[monthIndex] += v
					happyCount[monthIndex]++
				}
				if v, ok := scores["sad"]; ok {
					sadSum[monthIndex] += v
					sadCount[monthIndex]++
				}
				if v, ok := scores["angry"]; ok {
					angrySum[monthIndex] += v
					angryCount[monthIndex]++
				}
				if v, ok := scores["anxious"]; ok {
					anxiousSum[monthIndex] += v
					anxiousCount[monthIndex]++
				}
				if v, ok := scores["neutral"]; ok {
					neutralSum[monthIndex] += v
					neutralCount[monthIndex]++
				}
			}
		}
	}

	happyData := make([]int, months)
	sadData := make([]int, months)
	angryData := make([]int, months)
	anxiousData := make([]int, months)
	neutralData := make([]int, months)

	for i := 0; i < months; i++ {
		if happyCount[i] > 0 {
			happyData[i] = int(happySum[i] / float64(happyCount[i]) * 100)
		}
		if sadCount[i] > 0 {
			sadData[i] = int(sadSum[i] / float64(sadCount[i]) * 100)
		}
		if angryCount[i] > 0 {
			angryData[i] = int(angrySum[i] / float64(angryCount[i]) * 100)
		}
		if anxiousCount[i] > 0 {
			anxiousData[i] = int(anxiousSum[i] / float64(anxiousCount[i]) * 100)
		}
		if neutralCount[i] > 0 {
			neutralData[i] = int(neutralSum[i] / float64(neutralCount[i]) * 100)
		}
	}

	return []SeriesData{
		{Name: "开心指数", Data: happyData},
		{Name: "悲伤指数", Data: sadData},
		{Name: "愤怒指数", Data: angryData},
		{Name: "焦虑指数", Data: anxiousData},
		{Name: "中性指数", Data: neutralData},
	}
}
