package model

import (
	"testing"
	"time"
)

// TestConversation_TableName 验证表名
func TestConversation_TableName(t *testing.T) {
	c := Conversation{}
	if got := c.TableName(); got != "emotion_echo_chat.conversations" {
		t.Fatalf("want 'emotion_echo_chat.conversations' got %q", got)
	}
}

// TestConversation_DefaultValues 字段可被读写
func TestConversation_DefaultValues(t *testing.T) {
	now := time.Now()
	c := Conversation{
		ID:           1,
		UserID:       7,
		Title:        "Test",
		MessageCount: 0,
		Status:       1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if c.ID != 1 || c.UserID != 7 || c.Title != "Test" {
		t.Fatalf("field mismatch: %+v", c)
	}
	if c.Status != 1 {
		t.Fatalf("status mismatch")
	}
}

// TestMessage_TableName 验证消息表名
func TestMessage_TableName(t *testing.T) {
	m := Message{}
	if got := m.TableName(); got != "emotion_echo_chat.messages" {
		t.Fatalf("want 'emotion_echo_chat.messages' got %q", got)
	}
}

// TestMessage_Fields 表驱动
func TestMessage_Fields(t *testing.T) {
	now := time.Now()
	m := Message{
		ID:             1,
		ConversationID: 100,
		UserID:         7,
		Role:           "assistant",
		Content:        "Hello",
		ContentType:    "text",
		TokensUsed:     5,
		CreatedAt:      now,
	}
	if m.Role != "assistant" || m.Content != "Hello" {
		t.Fatalf("field mismatch: %+v", m)
	}
	if m.ContentType != "text" {
		t.Fatalf("default contentType should be 'text', got %q", m.ContentType)
	}
}

// TestConversation_LastMessageAtNullable LastMessageAt 是 *time.Time（可空）
func TestConversation_LastMessageAtNullable(t *testing.T) {
	c := Conversation{}
	if c.LastMessageAt != nil {
		t.Fatalf("expected nil pointer for nullable field")
	}
	now := time.Now()
	c.LastMessageAt = &now
	if c.LastMessageAt == nil {
		t.Fatalf("set LastMessageAt failed")
	}
}
