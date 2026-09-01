// Stage 34 · PR-5 RED
//
// FusedEmotion 是多模态情绪融合产物，对应 emotion_echo_ai.fused_emotions 表。
//
// Schema：
//   id                 BIGSERIAL PK
//   message_id         BIGINT NOT NULL UNIQUE   -- ★ 每条消息至多一个融合结果
//   user_id            BIGINT NOT NULL
//   conversation_id    BIGINT NOT NULL
//   primary_emotion    VARCHAR(32) NOT NULL
//   sentiment_score    REAL
//   confidence         REAL
//   modality_contrib   JSONB DEFAULT '{}'       -- {"text":0.4,"voice":0.3,"face":0.3}
//   reasoning          TEXT                      -- LLM 输出（走 late_fusion 时为空）
//   fusion_method      VARCHAR(32)               -- "llm" | "late_fusion_weighted"
//   available_modality TEXT[]                    -- ["text","voice","face"]
//   created_at         TIMESTAMPTZ
package model

import (
	"testing"
	"time"
)

func TestFusedEmotion_TableName(t *testing.T) {
	var f FusedEmotion
	if got := f.TableName(); got != "emotion_echo_ai.fused_emotions" {
		t.Fatalf("want 'emotion_echo_ai.fused_emotions' got %q", got)
	}
}

// TestFusedEmotion_Fields 字段读写 + UNIQUE message_id 由 DB 约束（gorm tag 验证）。
func TestFusedEmotion_Fields(t *testing.T) {
	now := time.Now()
	f := FusedEmotion{
		ID:               1,
		MessageID:        100,
		UserID:           7,
		ConversationID:   50,
		PrimaryEmotion:   "sad",
		SentimentScore:   -0.42,
		Confidence:       0.81,
		ModalityContrib:  `{"text":0.4,"voice":0.3,"face":0.3}`,
		Reasoning:        "用户文字与语音均表达低落情绪",
		FusionMethod:     "llm",
		AvailableModalities: []string{"text", "voice"},
		CreatedAt:        now,
	}
	if f.MessageID != 100 || f.UserID != 7 {
		t.Fatalf("ids mismatch")
	}
	if f.PrimaryEmotion != "sad" || f.Confidence != 0.81 {
		t.Fatalf("emotion fields mismatch")
	}
	if f.FusionMethod != "llm" {
		t.Fatalf("fusion_method mismatch")
	}
	if len(f.AvailableModalities) != 2 {
		t.Fatalf("available_modalities mismatch")
	}
}

// TestFusedEmotion_LateFusionReasoningEmpty reasoning 可空（走 late_fusion 时）。
func TestFusedEmotion_LateFusionReasoningEmpty(t *testing.T) {
	f := FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "neutral", FusionMethod: "late_fusion_weighted",
	}
	if f.Reasoning != "" {
		t.Fatalf("reasoning should default to empty for late_fusion")
	}
}
