// Package model 定义 assessment-svc 拥有的领域实体。
//
// 这些 struct 对应 emotion_echo_assessment schema 中的表。
// 跨服务查询必须通过 RPC，禁止跨 schema JOIN。
package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// JSONMap 是简单的 JSON 列类型（替代 gorm.io/gorm/datatypes）
type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	}
	if len(b) == 0 {
		*m = nil
		return nil
	}
	return json.Unmarshal(b, m)
}

// Survey 量表（与 emotion_echo_assessment.surveys 表对应）
type Survey struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Code         string    `gorm:"column:code;size:64;uniqueIndex"`
	Title        string    `gorm:"column:title;size:255"`
	Description  string    `gorm:"column:description"`
	Category     string    `gorm:"column:category;size:32"`
	Questions    JSONMap   `gorm:"column:questions;type:jsonb"`
	ScoringRules JSONMap   `gorm:"column:scoring_rules;type:jsonb"`
	Version      int       `gorm:"column:version;default:1"`
	Status       int16     `gorm:"column:status;default:1"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 显式指定 schema + 表名
func (Survey) TableName() string { return "emotion_echo_assessment.surveys" }

// SurveyResult 量表结果（emotion_echo_assessment.survey_results 表对应）
type SurveyResult struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID       int64     `gorm:"column:user_id;index"`
	SurveyID     uint64    `gorm:"column:survey_id"`
	Answers      JSONMap   `gorm:"column:answers;type:jsonb"`
	TotalScore   float64   `gorm:"column:total_score"`
	FactorScores JSONMap   `gorm:"column:factor_scores;type:jsonb"`
	RiskLevel    string    `gorm:"column:risk_level;size:32"`
	DurationSec  int       `gorm:"column:duration_seconds"`
	SubmittedAt  time.Time `gorm:"column:submitted_at;autoCreateTime"`
}

func (SurveyResult) TableName() string { return "emotion_echo_assessment.survey_results" }