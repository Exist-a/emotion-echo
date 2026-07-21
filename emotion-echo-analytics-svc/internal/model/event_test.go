package model

import (
	"testing"
	"time"
)

// TestUserBehaviorEvent_TableName 验证表名
func TestUserBehaviorEvent_TableName(t *testing.T) {
	e := UserBehaviorEvent{}
	if got := e.TableName(); got != "emotion_echo_analytics.user_behavior_events" {
		t.Fatalf("want 'emotion_echo_analytics.user_behavior_events' got %q", got)
	}
}

// TestUserBehaviorEvent_Fields 字段读写
func TestUserBehaviorEvent_Fields(t *testing.T) {
	now := time.Now()
	e := UserBehaviorEvent{
		ID:         1,
		UserID:     7,
		EventType:  "page_view",
		Target:     "/dashboard",
		SessionID:  "s-abc",
		OccurredAt: now,
	}
	if e.UserID != 7 || e.EventType != "page_view" || e.Target != "/dashboard" {
		t.Fatalf("field mismatch: %+v", e)
	}
	if e.SessionID != "s-abc" {
		t.Fatalf("session id mismatch")
	}
}

// TestUserBehaviorEvent_ZeroValue 表驱动零值安全
func TestUserBehaviorEvent_ZeroValue(t *testing.T) {
	e := UserBehaviorEvent{}
	if e.ID != 0 || e.UserID != 0 {
		t.Fatalf("zero event should be all zero")
	}
	if e.OccurredAt.IsZero() == false {
		t.Fatalf("zero event OccurredAt should be zero")
	}
}
