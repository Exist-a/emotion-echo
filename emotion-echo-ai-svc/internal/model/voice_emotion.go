// Package model — Stage 34 · PR-4 GREEN
//
// VoiceEmotionResult 是 SenseVoice（语音情绪识别）落库模型，
// 对应 emotion_echo_ai.voice_emotion_results 表。
//
// 与 FaceEmotionResult 同模式：
//   - UploadID UNIQUE 幂等
//   - MessageID 可空（用户可能上传失败 / 无语音消息关联）
//   - JSONB 字段 string 存 JSON 文本（与 emotion_analysis 一致）
package model

import "time"

// VoiceEmotionResult 单次 SenseVoice 分析的持久化产物。
//
// Transcript 可空：SenseVoice ASR 失败时只保留 emotion token。
// DurationMs / Language 用于产品侧"语速 / 多语种"分析（Stage 35+ 用）。
type VoiceEmotionResult struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UploadID       string    `gorm:"column:upload_id;size:64;uniqueIndex:uq_voice_emotion_upload_id"`
	MessageID      int64     `gorm:"column:message_id;index"`
	UserID         int64     `gorm:"column:user_id"`
	ConversationID int64     `gorm:"column:conversation_id"`
	Transcript     string    `gorm:"column:transcript;type:text"`
	PrimaryEmotion string    `gorm:"column:primary_emotion;size:32"`
	EmotionScores  string    `gorm:"column:emotion_scores;type:jsonb;default:'{}'"`
	Confidence     float64   `gorm:"column:confidence"`
	Model          string    `gorm:"column:model;size:64"`
	DurationMs     int       `gorm:"column:duration_ms"`
	Language       string    `gorm:"column:language;size:16"`
	RawResponse    string    `gorm:"column:raw_response;type:jsonb"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 显式指向 emotion_echo_ai schema。
func (VoiceEmotionResult) TableName() string { return "emotion_echo_ai.voice_emotion_results" }
