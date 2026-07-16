package models

import (
	"time"
)

// Survey 心理测验模型
type Survey struct {
	ID            int              `gorm:"primaryKey" json:"id"`
	Title         string           `gorm:"size:200;not null" json:"title"`
	Description   string           `gorm:"type:text" json:"description"`
	EstimatedTime string           `gorm:"size:50" json:"estimatedTime"`
	Questions     []SurveyQuestion `gorm:"type:jsonb" json:"questions"`
	CreatedAt     time.Time        `json:"createdAt"`
}

// TableName 表名
func (Survey) TableName() string {
	return "surveys"
}

// SurveyQuestion 测验题目
type SurveyQuestion struct {
	ID      int            `json:"id"`
	Title   string         `json:"title"`
	Type    string         `json:"type"`
	Options []SurveyOption `json:"options"`
}

// SurveyOption 选项
type SurveyOption struct {
	ID    int    `json:"id"`
	Text  string `json:"text"`
	Score int    `json:"score"`
}
