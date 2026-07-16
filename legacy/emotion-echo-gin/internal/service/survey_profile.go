package service

import (
	"context"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/repository"
)

// GetLatestPsychProfile 获取用户最新心理画像
func GetLatestPsychProfile(ctx context.Context, userID int64, resultRepo *repository.SurveyResultRepository) *PsychProfile {
	results, err := resultRepo.ListByUserID(ctx, userID)
	if err != nil || len(results) == 0 {
		return &PsychProfile{HasSurveyData: false}
	}

	profile := &PsychProfile{HasSurveyData: true}

	// 查找最新的SDS和SAS结果
	var latestSDS, latestSAS *models.SurveyResult
	for _, r := range results {
		if r.SurveyID == 1 && (latestSDS == nil || r.CreatedAt.After(latestSDS.CreatedAt)) {
			latestSDS = r
		}
		if r.SurveyID == 2 && (latestSAS == nil || r.CreatedAt.After(latestSAS.CreatedAt)) {
			latestSAS = r
		}
	}

	if latestSDS != nil {
		profile.SDS = &ScaleProfile{
			Score:       latestSDS.TotalScore,
			Level:       latestSDS.Level,
			CompletedAt: latestSDS.CreatedAt.Format("2006-01-02"),
		}
		profile.LatestSurveyDate = latestSDS.CreatedAt.Format("2006-01-02")
	}

	if latestSAS != nil {
		profile.SAS = &ScaleProfile{
			Score:       latestSAS.TotalScore,
			Level:       latestSAS.Level,
			CompletedAt: latestSAS.CreatedAt.Format("2006-01-02"),
		}
		if latestSDS == nil || latestSAS.CreatedAt.After(latestSDS.CreatedAt) {
			profile.LatestSurveyDate = latestSAS.CreatedAt.Format("2006-01-02")
		}
	}

	return profile
}
