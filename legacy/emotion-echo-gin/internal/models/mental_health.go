package models

import "time"

// MentalHealthAssessment 心理健康评估
type MentalHealthAssessment struct {
	ID              int64     `gorm:"primaryKey" json:"id"`
	UserID          int64     `gorm:"index;not null" json:"-"`
	
	AssessmentType  string    `gorm:"size:20;not null" json:"assessmentType"`
	PeriodStart     time.Time `json:"periodStart"`
	PeriodEnd       time.Time `json:"periodEnd"`
	
	EmotionScore    int       `json:"emotionScore"`
	DepressionScore int       `json:"depressionScore"`
	AnxietyScore    int       `json:"anxietyScore"`
	StressScore     int       `json:"stressScore"`
	SleepScore      int       `json:"sleepScore"`
	SocialScore     int       `json:"socialScore"`
	
	OverallScore    int       `json:"overallScore"`
	RiskLevel       string    `gorm:"size:20;not null" json:"riskLevel"`
	RiskFactors     []string  `gorm:"type:jsonb" json:"riskFactors"`
	WarningFlags    []string  `gorm:"type:jsonb" json:"warningFlags"`
	
	Summary         string    `json:"summary"`
	Suggestions     []Suggestion `gorm:"type:jsonb" json:"suggestions"`
	
	EmotionAnalysisIDs []int64  `gorm:"type:jsonb" json:"-"`
	SurveyResultIDs    []string `gorm:"type:jsonb" json:"-"`
	
	IsNotified      bool      `json:"isNotified"`
	CreatedAt       time.Time `json:"createdAt"`
}

// TableName 表名
func (MentalHealthAssessment) TableName() string {
	return "mental_health_assessments"
}

// ToDimensionScores 转换为维度评分列表（用于前端展示）
func (a *MentalHealthAssessment) ToDimensionScores() []DimensionScore {
	return []DimensionScore{
		{Name: "情绪稳定", Score: a.EmotionScore, Max: 100},
		{Name: "抑郁风险", Score: a.DepressionScore, Max: 100},
		{Name: "焦虑风险", Score: a.AnxietyScore, Max: 100},
		{Name: "压力指数", Score: a.StressScore, Max: 100},
		{Name: "睡眠质量", Score: a.SleepScore, Max: 100},
		{Name: "社交活力", Score: a.SocialScore, Max: 100},
	}
}

// Suggestion 干预建议
type Suggestion struct {
	Level     string   `json:"level"`     // immediate / short_term / long_term
	Category  string   `json:"category"`  // professional / self_help / lifestyle
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Actions   []string `json:"actions"`
}

// DimensionScore 单维度评分
type DimensionScore struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Max   int    `json:"max"`
}
