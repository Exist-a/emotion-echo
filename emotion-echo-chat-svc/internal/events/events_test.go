package events

import (
	"context"
	"testing"
	"time"
)

// TestTopicConstants 验证 topic 常量值
func TestTopicConstants(t *testing.T) {
	if TopicChatEvents != "chat-events" {
		t.Fatalf("TopicChatEvents want 'chat-events' got %q", TopicChatEvents)
	}
}

// TestEventTypeConstants 表驱动：验证 3 个事件类型常量
func TestEventTypeConstants(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{EventTypeConversationCreated, "conversation.created"},
		{EventTypeConversationClosed, "conversation.closed"},
		{EventTypeMessageCreated, "message.created"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("event type %q want %q", tc.got, tc.want)
		}
	}
}

// TestInMemoryEventPublisher_PublishAndRetrieve happy path
func TestInMemoryEventPublisher_PublishAndRetrieve(t *testing.T) {
	p := NewInMemoryEventPublisher()
	defer p.Close()
	e := &Event{
		ID:   "evt-1",
		Type: EventTypeMessageCreated,
		Time: time.Now().UTC(),
		Data: MessageCreatedData{ConversationID: 1, UserID: 1},
	}
	if err := p.Publish(context.Background(), TopicChatEvents, e); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got := p.Events(TopicChatEvents)
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	if got[0].ID != "evt-1" {
		t.Fatalf("id mismatch: %q", got[0].ID)
	}
}

// TestInMemoryEventPublisher_NoDataForTopic 未发布过的 topic 返回空 slice
func TestInMemoryEventPublisher_NoDataForTopic(t *testing.T) {
	p := NewInMemoryEventPublisher()
	defer p.Close()
	got := p.Events("never-published-topic")
	if len(got) != 0 {
		t.Fatalf("want 0 events for unknown topic, got %d", len(got))
	}
}

// TestInMemoryEventPublisher_MultipleEvents 表驱动：3 个事件
func TestInMemoryEventPublisher_MultipleEvents(t *testing.T) {
	p := NewInMemoryEventPublisher()
	defer p.Close()

	cases := []struct {
		topic string
		typ   string
	}{
		{TopicChatEvents, EventTypeMessageCreated},
		{TopicChatEvents, EventTypeConversationCreated},
		{"another.topic", EventTypeConversationClosed},
	}
	for i, tc := range cases {
		e := &Event{
			ID:   "id",
			Type: tc.typ,
			Time: time.Now(),
		}
		if err := p.Publish(context.Background(), tc.topic, e); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	if got := len(p.Events(TopicChatEvents)); got != 2 {
		t.Fatalf("TopicChatEvents want 2 got %d", got)
	}
	if got := len(p.Events("another.topic")); got != 1 {
		t.Fatalf("another.topic want 1 got %d", got)
	}
}

// TestInMemoryEventPublisher_EventsReturnsCopy **Stage 26-N 修复后**：Events 必须深拷贝 Event
//
// 历史：Stage 26-A 暴露 Events 仅 shallow-copy，外部修改会影响内部。
func TestInMemoryEventPublisher_EventsReturnsCopy(t *testing.T) {
	p := NewInMemoryEventPublisher()
	defer p.Close()
	e := &Event{ID: "1", Type: EventTypeMessageCreated}
	if err := p.Publish(context.Background(), TopicChatEvents, e); err != nil {
		t.Fatalf("publish: %v", err)
	}
	out := p.Events(TopicChatEvents)
	out[0].ID = "MODIFIED"

	again := p.Events(TopicChatEvents)
	if again[0].ID == "MODIFIED" {
		t.Fatalf("internal Event got mutated: ID=%q (Events should deep-copy Event)", again[0].ID)
	}
}

// TestInMemoryEventPublisher_Close_NilOp Close 不应有副作用
func TestInMemoryEventPublisher_Close_NilOp(t *testing.T) {
	p := NewInMemoryEventPublisher()
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// 多次 Close 也安全
	for i := 0; i < 3; i++ {
		if err := p.Close(); err != nil {
			t.Fatalf("Close %d: %v", i, err)
		}
	}
}

// TestMessageCreatedData_Fields 表驱动：JSON tag 期望值
func TestMessageCreatedData_Fields(t *testing.T) {
	data := MessageCreatedData{
		MessageID:      10,
		ConversationID: 20,
		UserID:         100,
		Role:           "user",
		Content:        "hi",
		CreatedAt:      1700000000,
	}
	if data.MessageID != 10 || data.Role != "user" || data.Content != "hi" {
		t.Fatalf("field mismatch: %+v", data)
	}
}

// TestEvent_Fields 验证 Event 能正常构造
func TestEvent_Fields(t *testing.T) {
	e := &Event{
		ID:     "u-1",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Now(),
		Data:   MessageCreatedData{UserID: 99},
	}
	if e.Source != "chat-svc" {
		t.Fatalf("source mismatch")
	}
	if e.Type != EventTypeMessageCreated {
		t.Fatalf("type mismatch")
	}
}
