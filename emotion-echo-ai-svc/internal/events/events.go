// Package events 定义 ai-svc 消费的事件 schema
//
// 与 chat-svc 的 events 包结构对齐（生产/消费双方约定）
package events

import "time"

// Topic 是 ai-svc 订阅的 topic
const (
	// TopicChatEvents 是 chat-svc 产生的所有事件
	TopicChatEvents = "chat-events"
)

// EventType 是 ai-svc 关心的事件类型
const (
	EventTypeMessageCreated = "message.created"
)

// Event 是 chat-svc 产生的事件结构（与 chat-svc 的 events.Event 同构）
type Event struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"`
	Source string    `json:"source"`
	Time   time.Time `json:"time"`
	Data   any       `json:"data"`
}

// MessageCreatedData 来自 chat-svc 的 message.created 事件载荷
//
// JSON 反序列化：chat-svc 用 snake_case (camelCase)，这里保持一致
type MessageCreatedData struct {
	MessageID      int64  `json:"messageId"`
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"createdAt"`
}