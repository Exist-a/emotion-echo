package models

import (
	"time"
)

// SurveyResult 测验结果模型
type SurveyResult struct {
	ID         string       `gorm:"primaryKey;size:32" json:"resultId"`
	UserID     int64        `gorm:"index;not null" json:"-"`
	SurveyID   int          `gorm:"not null" json:"-"`
	Answers    []UserAnswer `gorm:"type:jsonb" json:"-"`
	TotalScore int          `json:"totalScore"`
	Level      string       `gorm:"size:50" json:"level"`
	Suggestion string       `gorm:"type:text" json:"suggestion"`
	CreatedAt  time.Time    `json:"createdAt"`
}

// TableName 表名
func (SurveyResult) TableName() string {
	return "survey_results"
}

// UserAnswer 用户答案
type UserAnswer struct {
	QuestionID int `json:"questionId"`
	OptionID   int `json:"optionId"`
}
