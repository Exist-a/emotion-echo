// Stage 34 · PR-1 RED
//
// FaceEmotionResult 是 FER（人脸情绪识别）的落库模型。
//
// Schema（emotion_echo_ai.face_emotion_results）：
//   id              BIGSERIAL PK
//   upload_id       VARCHAR(64) UNIQUE         -- 前端去重 nonce
//   message_id      BIGINT                     -- 可空（用户可能上传无人脸）
//   user_id         BIGINT NOT NULL
//   conversation_id BIGINT
//   primary_emotion VARCHAR(32)
//   emotion_scores  JSONB
//   confidence      REAL
//   model           VARCHAR(64)
//   raw_response    JSONB
//   created_at      TIMESTAMPTZ
//
// 这些测试**故意**只引用未定义的符号 — 跑必须红。
package model

import (
	"testing"
	"time"
)

// TestFaceEmotionResult_TableName 表名必须指向 emotion_echo_ai.face_emotion_results。
func TestFaceEmotionResult_TableName(t *testing.T) {
	var f FaceEmotionResult
	if got := f.TableName(); got != "emotion_echo_ai.face_emotion_results" {
		t.Fatalf("want 'emotion_echo_ai.face_emotion_results' got %q", got)
	}
}

// TestFaceEmotionResult_Fields 字段读写 + UploadID 唯一。
func TestFaceEmotionResult_Fields(t *testing.T) {
	now := time.Now()
	f := FaceEmotionResult{
		ID:             42,
		UploadID:       "upload-nonce-001",
		MessageID:      100,
		UserID:         7,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		Confidence:     0.88,
		Model:          "fer:fer",
		CreatedAt:      now,
	}
	if f.UploadID != "upload-nonce-001" {
		t.Fatalf("upload_id mismatch: %q", f.UploadID)
	}
	if f.PrimaryEmotion != "happy" || f.Confidence != 0.88 {
		t.Fatalf("emotion fields mismatch: %+v", f)
	}
	if f.MessageID != 100 || f.UserID != 7 || f.ConversationID != 50 {
		t.Fatalf("ids mismatch")
	}
}

// TestFaceEmotionResult_MessageIDOptional message_id 必须可空（用户上传无人脸）。
func TestFaceEmotionResult_MessageIDOptional(t *testing.T) {
	f := FaceEmotionResult{UserID: 7, PrimaryEmotion: "neutral", Model: "fer:opencv-dnn"}
	if f.MessageID != 0 {
		t.Fatalf("expected zero MessageID for upload-without-chat, got %d", f.MessageID)
	}
}
