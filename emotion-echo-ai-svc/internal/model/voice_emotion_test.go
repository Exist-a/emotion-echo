// Stage 34 · PR-3 RED
//
// VoiceEmotionResult 是 SenseVoice（语音情绪识别）的落库模型。
//
// Schema（emotion_echo_ai.voice_emotion_results）：
//   id              BIGSERIAL PK
//   upload_id       VARCHAR(64) UNIQUE
//   message_id      BIGINT                     -- 可空
//   user_id         BIGINT NOT NULL
//   conversation_id BIGINT
//   transcript      TEXT                       -- SenseVoice 转写文本
//   primary_emotion VARCHAR(32)                -- SenseVoice 情绪 token
//   emotion_scores  JSONB
//   confidence      REAL
//   model           VARCHAR(64)
//   duration_ms     INT
//   language        VARCHAR(16)
//   raw_response    JSONB
//   created_at      TIMESTAMPTZ
package model

import (
	"testing"
	"time"
)

// TestVoiceEmotionResult_TableName
func TestVoiceEmotionResult_TableName(t *testing.T) {
	var v VoiceEmotionResult
	if got := v.TableName(); got != "emotion_echo_ai.voice_emotion_results" {
		t.Fatalf("want 'emotion_echo_ai.voice_emotion_results' got %q", got)
	}
}

// TestVoiceEmotionResult_Fields 字段读写。
func TestVoiceEmotionResult_Fields(t *testing.T) {
	now := time.Now()
	v := VoiceEmotionResult{
		ID:             1,
		UploadID:       "voice-nonce-001",
		MessageID:      200,
		UserID:         7,
		ConversationID: 50,
		Transcript:     "我今天心情不太好",
		PrimaryEmotion: "sad",
		Confidence:     0.92,
		Model:          "sensevoice:sensevoice-small",
		DurationMs:     5400,
		Language:       "zh",
		CreatedAt:      now,
	}
	if v.Transcript != "我今天心情不太好" || v.PrimaryEmotion != "sad" {
		t.Fatalf("emotion fields mismatch")
	}
	if v.DurationMs != 5400 || v.Language != "zh" {
		t.Fatalf("audio metadata mismatch")
	}
}

// TestVoiceEmotionResult_TranscriptOptional 转写失败时 transcript 可空。
func TestVoiceEmotionResult_TranscriptOptional(t *testing.T) {
	v := VoiceEmotionResult{
		UserID: 7, PrimaryEmotion: "neutral", Model: "sensevoice:sensevoice-small",
	}
	if v.Transcript != "" {
		t.Fatalf("transcript should default to empty")
	}
}
