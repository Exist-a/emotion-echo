// Package model — security_answer.go
//
// E2E-06: 密保问题模型（供 D-01=C 找回密码）
package model

import "time"

// SecurityAnswer 与 emotion_echo_user.user_security_answers 表对应。
// 支持每个用户 1~2 个密保问题（question_order = 1 或 2）。
type SecurityAnswer struct {
	UserID       int64     `gorm:"column:user_id;primaryKey"`
	QuestionOrder int16    `gorm:"column:question_order;primaryKey"`
	Question     string    `gorm:"column:question;size:255"`
	AnswerHash   string    `gorm:"column:answer_hash;size:255"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 显式指定 schema + 表名
func (SecurityAnswer) TableName() string {
	return "emotion_echo_user.user_security_answers"
}