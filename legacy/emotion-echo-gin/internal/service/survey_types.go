package service

import (
	"time"

	"emotion-echo-gin/internal/models"
)

// SubmitRequest 提交答案请求
type SubmitRequest struct {
	Answers []models.UserAnswer `json:"answers" binding:"required"`
}

// SubmitResult 提交结果
type SubmitResult struct {
	ResultID      string `json:"resultId"`
	RawScore      int    `json:"rawScore"`
	TotalScore    int    `json:"totalScore"` // 标准分（SDS/SAS: raw × 1.25）
	Level         string `json:"level"`
	Suggestion    string `json:"suggestion"`
}

// PsychProfile 用户心理画像（用于AI上下文）
type PsychProfile struct {
	HasSurveyData    bool `json:"hasSurveyData"`
	LatestSurveyDate string `json:"latestSurveyDate"`
	SDS             *ScaleProfile `json:"sds,omitempty"`
	SAS             *ScaleProfile `json:"sas,omitempty"`
}

// ScaleProfile 单个量表结果
type ScaleProfile struct {
	Score     int `json:"score"`     // 标准分
	RawScore  int `json:"rawScore"`  // 原始分
	Level     string `json:"level"`     // 等级
	CompletedAt string `json:"completedAt"`
}

// SurveyListItem 量表列表项
type SurveyListItem struct {
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	EstimatedTime string     `json:"estimatedTime"`
	Status        string     `json:"status"` // not_started | completed
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	ResultID      string     `json:"resultId,omitempty"`
}
