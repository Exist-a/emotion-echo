package events

import (
	"encoding/json"
	"testing"
	"time"
)

// TestTopicConstants topic 常量值
func TestTopicConstants(t *testing.T) {
	if TopicChatEvents != "chat-events" {
		t.Fatalf("want 'chat-events' got %q", TopicChatEvents)
	}
}

// TestEventTypeConstants
func TestEventTypeConstants(t *testing.T) {
	if EventTypeMessageCreated != "message.created" {
		t.Fatalf("want 'message.created' got %q", EventTypeMessageCreated)
	}
}

// TestEvent_JSONRoundTrip JSON tag round-trip
func TestEvent_JSONRoundTrip(t *testing.T) {
	e := Event{
		ID:     "evt-1",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Now().UTC(),
		Data:   "any-data",
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Event
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != e.ID || back.Type != e.Type || back.Source != e.Source {
		t.Fatalf("round-trip lost field(s)")
	}
}

// TestMessageCreatedData_JSONRoundTrip 表驱动字段
func TestMessageCreatedData_JSONRoundTrip(t *testing.T) {
	d := MessageCreatedData{
		MessageID:      11,
		ConversationID: 22,
		UserID:         33,
		Role:           "user",
		Content:        "hi",
		CreatedAt:      1700000000,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(b, "messageId") {
		t.Fatalf("expected camelCase field: %s", b)
	}
	var back MessageCreatedData
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.MessageID != d.MessageID || back.Content != d.Content {
		t.Fatalf("round-trip mismatch: %+v", back)
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
