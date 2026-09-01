// Package model — Stage 34 · PR-6 GREEN
//
// FusedEmotion 是多模态情绪融合产物，对应 emotion_echo_ai.fused_emotions 表。
//
// 核心约束：message_id UNIQUE —— 每条消息至多一个融合结果。
// Worker 在 face/voice 数据陆续到达时会 Upsert 覆盖（不是新建）。
package model

import (
	"time"

	"gorm.io/gorm"
)

// FusedEmotion 多模态融合产物。
//
// ModalityContrib 是 JSON 字符串（如 {"text":0.4,"voice":0.3,"face":0.3}），
// 与 emotion_scores 同样以 string 存 JSONB（与现有 emotion_analysis 一致）。
//
// FusionMethod 决定 Reasoning 是否可空：
//   - "llm" → Reasoning 非空（LLM 输出）
//   - "late_fusion_weighted" → Reasoning 为空
//
// AvailableModalities 记录本次融合实际用了哪几路（用于前端展示"基于 X 路"）。
type FusedEmotion struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement"`
	MessageID          int64     `gorm:"column:message_id;uniqueIndex:uq_fused_emotions_message_id;not null"`
	UserID             int64     `gorm:"column:user_id;not null"`
	ConversationID     int64     `gorm:"column:conversation_id;not null"`
	PrimaryEmotion     string    `gorm:"column:primary_emotion;size:32;not null"`
	SentimentScore     float64   `gorm:"column:sentiment_score"`
	Confidence         float64   `gorm:"column:confidence"`
	ModalityContrib    string    `gorm:"column:modality_contrib;type:jsonb;default:'{}'"`
	Reasoning          string    `gorm:"column:reasoning;type:text"`
	FusionMethod       string    `gorm:"column:fusion_method;size:32"`
	AvailableModalities []string `gorm:"column:available_modalities;type:text[]"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 显式指向 emotion_echo_ai schema。
func (FusedEmotion) TableName() string { return "emotion_echo_ai.fused_emotions" }

// BeforeCreate 是 gorm 钩子占位（暂不实现，预留扩展点）。
//
// 当前由 repo 层显式控制 ON CONFLICT；如未来需要更复杂逻辑（如
// 拒绝 message_id=0 的记录），可在此处添加校验。
func (f *FusedEmotion) BeforeCreate(tx *gorm.DB) error {
	return nil
}
