// Package model 提供 ai-svc 拥有的领域实体。
//
// Stage 34 · PR-2 GREEN: FaceEmotionResult 是 FER（人脸情绪识别）落库模型，
// 对应 emotion_echo_ai.face_emotion_results 表。
package model

import "time"

// FaceEmotionResult 单次 FER 分析的持久化产物。
//
// UploadID 是前端上传去重 nonce，DB 上挂 UNIQUE 约束；repo 层
// ON CONFLICT (upload_id) DO NOTHING 实现幂等。
//
// MessageID 可空：用户可能上传无人脸帧（系统返回 neutral），
// 此时不入会话情绪链路，只入用户行为分析。
type FaceEmotionResult struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UploadID       string    `gorm:"column:upload_id;size:64;uniqueIndex:uq_face_emotion_upload_id"`
	MessageID      int64     `gorm:"column:message_id;index"`
	UserID         int64     `gorm:"column:user_id"`
	ConversationID int64     `gorm:"column:conversation_id"`
	PrimaryEmotion string    `gorm:"column:primary_emotion;size:32"`
	EmotionScores  string    `gorm:"column:emotion_scores;type:jsonb;default:'{}'"`
	Confidence     float64   `gorm:"column:confidence"`
	Model          string    `gorm:"column:model;size:64"`
	RawResponse    string    `gorm:"column:raw_response;type:jsonb"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 显式指向 emotion_echo_ai schema，避免 search_path 漂移。
func (FaceEmotionResult) TableName() string { return "emotion_echo_ai.face_emotion_results" }
