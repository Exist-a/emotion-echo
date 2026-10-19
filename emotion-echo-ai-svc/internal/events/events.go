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
	// EventTypeConversationClosed CL-08a：会话收尾（混合粒度的合并/失效判定触发）。
	// 载荷与 chat-svc 的 ConversationClosedData 同构。
	EventTypeConversationClosed = "conversation.closed"
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

// ConversationClosedData 来自 chat-svc 的 conversation.closed 事件载荷。
//
// CL-08a 混合粒度的第二跳：对该会话已抽事实做一次确定性合并/失效判定。
type ConversationClosedData struct {
	ConversationID int64 `json:"conversationId"`
	UserID         int64 `json:"userId"`
	ClosedAt       int64 `json:"closedAt"`
}
