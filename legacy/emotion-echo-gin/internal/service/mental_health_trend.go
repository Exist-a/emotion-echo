package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/repository"
)

// GetTrendData 获取趋势数据
func GetTrendData(ctx context.Context, userID int64, trendType string, assessmentRepo *repository.MentalHealthRepository) (*TrendData, error) {
	// 确定时间范围
	var days int
	switch trendType {
	case "monthly":
		days = 30
	default:
		days = 7
		trendType = "weekly"
	}

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days+1)

	// 查询评估记录
	assessments, err := assessmentRepo.ListByDateRange(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// 生成日期标签
	dates := make([]string, days)
	for i := 0; i < days; i++ {
		d := endDate.AddDate(0, 0, -(days - 1 - i))
		dates[i] = d.Format("01-02")
	}

	// 初始化分数数组
	scores := map[string][]int{
		"overall":    make([]int, days),
		"emotion":    make([]int, days),
		"depression": make([]int, days),
		"anxiety":    make([]int, days),
		"stress":     make([]int, days),
		"sleep":      make([]int, days),
		"social":     make([]int, days),
	}

	// 填充数据
	for _, a := range assessments {
		dayIndex := int(a.CreatedAt.Sub(startDate).Hours() / 24)
		if dayIndex >= 0 && dayIndex < days {
			scores["overall"][dayIndex] = a.OverallScore
			scores["emotion"][dayIndex] = a.EmotionScore
			scores["depression"][dayIndex] = a.DepressionScore
			scores["anxiety"][dayIndex] = a.AnxietyScore
			scores["stress"][dayIndex] = a.StressScore
			scores["sleep"][dayIndex] = a.SleepScore
			scores["social"][dayIndex] = a.SocialScore
		}
	}

	return &TrendData{
		Type:   trendType,
		Dates:  dates,
		Scores: scores,
	}, nil
}
