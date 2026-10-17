// Package model — user_agent_profile.go
//
// CL-06（用户自定义 Agent·多角色）：角色档案模型。
//
// 一个用户最多 5 个角色（含默认「小E」）；软删除（`deleted_at`），
// 与既有 c008/c009 软删除纪律一致。`boundaries` 是负向约束清单（JSONB 字符串数组）。
package model

import "time"

// UserAgentProfile 与 emotion_echo_user.user_agent_profile 表对应。
//
// 字段上限（保存时校验，见计划 §4）：Name ≤20 / Persona ≤500 / Tone ≤100 /
// Boundaries ≤5 条且每条 ≤50 / Voice ∈ 8 个 CosyVoice2 预置音色。
type UserAgentProfile struct {
	ID     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	UserID int64  `gorm:"column:user_id;index"`
	Name   string `gorm:"column:name;size:32"`
	// Gender 仅用于展示，不参与安全判定
	Gender  string `gorm:"column:gender;size:8"`
	Persona string `gorm:"column:persona"`
	Tone    string `gorm:"column:tone;size:128"`
	// Boundaries 负向约束（"不要…"），JSONB 字符串数组
	Boundaries []string `gorm:"column:boundaries;type:jsonb;serializer:json"`
	// Voice = 预置音色，带模型名前缀（如 FunAudioLLM/CosyVoice2-0.5B:anna）
	Voice      string `gorm:"column:voice;size:64"`
	IsDefault  bool   `gorm:"column:is_default;default:false"`
	TemplateID string `gorm:"column:template_id;size:32"`

	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index"`
}

// TableName 显式指定 schema + 表名
func (UserAgentProfile) TableName() string {
	return "emotion_echo_user.user_agent_profile"
}
