package types

import (
	"encoding/json"
	"testing"
)

// TestEmotionView_FieldJsonTags JSON tag 期望值
func TestEmotionView_FieldJsonTags(t *testing.T) {
	v := EmotionView{
		Id: 1, MessageId: 100, ConversationId: 50, UserId: 7,
		PrimaryEmotion: "happy", SentimentScore: 0.5, Confidence: 0.8,
		Model: "x", CreatedAt: 1700000000,
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{"messageId", "primaryEmotion", "sentimentScore", "confidence"} {
		if !contains(b, want) {
			t.Fatalf("expected %q in JSON: %s", want, b)
		}
	}
	var back EmotionView
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Id != v.Id || back.PrimaryEmotion != v.PrimaryEmotion {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}

// TestHealthResp_FieldJsonTags 表驱动校验 status / time / service / version / dbOK
func TestHealthResp_FieldJsonTags(t *testing.T) {
	r := HealthResp{
		Status: "ok", Time: 1700000000, Service: "ai-svc", Version: "0.1.0", DbOK: true,
	}
	b, _ := json.Marshal(r)
	for _, want := range []string{"status", "time", "service", "version", "dbOk"} {
		if !contains(b, want) {
			t.Fatalf("expected %q in JSON: %s", want, b)
		}
	}
}

// TestGetEmotionByMessageReq_Path 路由 tag
func TestGetEmotionByMessageReq_Path(t *testing.T) {
	r := GetEmotionByMessageReq{MessageId: 42}
	if r.MessageId != 42 {
		t.Fatalf("field mismatch")
	}
}

func contains(haystack []byte, needle string) bool {
	s := string(haystack)
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
