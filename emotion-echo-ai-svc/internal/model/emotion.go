// Package model 定义 ai-svc 拥有的领域实体。
package model

import "time"

// EmotionAnalysis 情绪分析（emotion_echo_ai.emotion_analysis 表对应）
//
// EventID 用于消费幂等：来自 chat-svc 发布的 Kafka 事件 ID。
// 写入数据库时由 repo 层 ON CONFLICT (event_id) DO NOTHING 去重，
// 保证 at-least-once 投递下同一事件不会落两行。空 EventID（来自
// gRPC 同步分析路径）走非去重路径。
type EmotionAnalysis struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventID        string    `gorm:"column:event_id;size:64;uniqueIndex:uq_emotion_analysis_event_id"`
	MessageID      int64     `gorm:"column:message_id;index"`
	UserID         int64     `gorm:"column:user_id"`
	ConversationID int64     `gorm:"column:conversation_id"`
	PrimaryEmotion string    `gorm:"column:primary_emotion;size:32"`
	SentimentScore float64   `gorm:"column:sentiment_score"`
	Confidence     float64   `gorm:"column:confidence"`
	Model          string    `gorm:"column:model;size:64"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (EmotionAnalysis) TableName() string { return "emotion_echo_ai.emotion_analysis" }

// VoiceTranscript 语音转写
type VoiceTranscript struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID     int64     `gorm:"column:user_id"`
	Transcript string    `gorm:"column:transcript"`
	Language   string    `gorm:"column:language;size:16"`
	Model      string    `gorm:"column:model;size:64"`
	Confidence float64   `gorm:"column:confidence"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (VoiceTranscript) TableName() string { return "emotion_echo_ai.voice_transcripts" }