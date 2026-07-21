package model

import (
	"testing"
	"time"
)

// TestEmotionAnalysis_Fields 表驱动字段读写
func TestEmotionAnalysis_Fields(t *testing.T) {
	now := time.Now()
	e := EmotionAnalysis{
		ID:             1,
		MessageID:      100,
		ConversationID: 50,
		UserID:         7,
		PrimaryEmotion: "happy",
		Confidence:     0.85,
		SentimentScore: 0.5,
		Model:          "keyword-stub-v1",
		CreatedAt:      now,
	}
	if e.PrimaryEmotion != "happy" || e.Confidence != 0.85 {
		t.Fatalf("field mismatch: %+v", e)
	}
	if e.Model != "keyword-stub-v1" {
		t.Fatalf("model mismatch")
	}
	if e.MessageID != 100 || e.UserID != 7 {
		t.Fatalf("ids mismatch")
	}
}

// TestEmotionAnalysis_TableName 表名校验
func TestEmotionAnalysis_TableName(t *testing.T) {
	e := EmotionAnalysis{}
	if got := e.TableName(); got != "emotion_echo_ai.emotion_analysis" {
		t.Fatalf("want 'emotion_echo_ai.emotion_analysis' got %q", got)
	}
}

// TestEmotionAnalysis_ZeroValue 表驱动零值
func TestEmotionAnalysis_ZeroValue(t *testing.T) {
	e := EmotionAnalysis{}
	if e.ID != 0 || e.MessageID != 0 || e.UserID != 0 {
		t.Fatalf("zero should yield all zero")
	}
	if e.PrimaryEmotion != "" || e.Model != "" {
		t.Fatalf("zero strings should be empty")
	}
}

// TestVoiceTranscript_TableName
func TestVoiceTranscript_TableName(t *testing.T) {
	v := VoiceTranscript{}
	if got := v.TableName(); got != "emotion_echo_ai.voice_transcripts" {
		t.Fatalf("want 'emotion_echo_ai.voice_transcripts' got %q", got)
	}
}

// TestVoiceTranscript_Fields
func TestVoiceTranscript_Fields(t *testing.T) {
	now := time.Now()
	v := VoiceTranscript{
		ID: 1, UserID: 7, Transcript: "hello", Language: "zh", Model: "sv",
		Confidence: 0.9, CreatedAt: now,
	}
	if v.Transcript != "hello" || v.Language != "zh" {
		t.Fatalf("field mismatch")
	}
}
