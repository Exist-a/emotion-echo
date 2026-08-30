// Package events — events.go
//
// Stage 30-A: chat-events Kafka 事件的本地镜像。
//
// chat-svc 的 internal/events 包定义了同样的 schema（id / type /
// source / time / data）。两个包是字面一致但不在同一模块 — 这是
// 故意：每个 svc 自治，事件 schema 变更要双 commit 同步。
//
// 此文件定义：
//   - Event / EventType* 常量
//   - MessageCreatedData / ConversationCreatedData /
//     ConversationClosedData 载荷结构
//   - TopicChatEvents 常量
package events

import "time"

// TopicChatEvents chat-svc 产生的所有事件的目标 topic
const TopicChatEvents = "chat-events"

// EventType 事件类型常量
const (
	EventTypeConversationCreated = "conversation.created"
	EventTypeConversationClosed  = "conversation.closed"
	EventTypeMessageCreated      = "message.created"
)

// Event 是 chat-svc 产生的所有事件的统一结构
//
// JSON 序列化结构（消费者按此解析）：
//   {
//     "id": "uuid-v4",
//     "type": "message.created",
//     "source": "chat-svc",
//     "time": "2026-07-13T12:00:00Z",
//     "data": {...}
//   }
type Event struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"`
	Source string    `json:"source"`
	Time   time.Time `json:"time"`
	Data   any       `json:"data"`
}

// MessageCreatedData 是 message.created 事件的载荷
type MessageCreatedData struct {
	MessageID      int64  `json:"messageId"`
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"createdAt"`
}

// ConversationCreatedData
type ConversationCreatedData struct {
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Title          string `json:"title"`
	CreatedAt      int64  `json:"createdAt"`
}

// ConversationClosedData
type ConversationClosedData struct {
	ConversationID int64 `json:"conversationId"`
	UserID         int64 `json:"userId"`
	ClosedAt       int64 `json:"closedAt"`
}