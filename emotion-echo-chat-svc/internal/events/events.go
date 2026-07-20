// Package events 定义 chat-svc 产生的事件 schema 和发布接口
//
// 设计原则：
//   - 借鉴 CloudEvents 规范（id/source/type/time/data）
//   - topic 常量集中管理，避免散落字符串
//   - EventPublisher 接口 → InMemoryEventPublisher（测试）+ KafkaEventPublisher（生产）
package events

import (
	"context"
	"sync"
	"time"
)

// Topic 是 Kafka topic 名称常量
//
// 跨服务契约：topic 名必须全集群一致，故放在独立包以避免循环依赖
const (
	// TopicChatEvents 是 chat-svc 产生的所有事件的目标 topic
	//
	// 订阅方：ai-svc（消费 message.created 做情绪分析）
	//        analytics-svc（消费全部事件做用户行为分析）
	TopicChatEvents = "chat-events"
)

// EventType 是事件类型
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
	// ID 事件唯一标识（UUID v4），用于消费者去重
	ID string `json:"id"`
	// Type 事件类型（用 EventType* 常量）
	Type string `json:"type"`
	// Source 事件源服务名
	Source string `json:"source"`
	// Time 事件产生时间
	Time time.Time `json:"time"`
	// Data 事件载荷（每个 Type 对应不同结构）
	Data any `json:"data"`
}

// MessageCreatedData 是 message.created 事件的载荷
//
// 消费者（ai-svc）会读 Content 做情绪分析
// ConversationID + UserID 用于关联数据
// MessageID 用于 ai-svc 写 emotion_analysis 时回填
type MessageCreatedData struct {
	MessageID      int64  `json:"messageId"`
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"createdAt"`
}

// ConversationCreatedData 是 conversation.created 事件的载荷
type ConversationCreatedData struct {
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Title          string `json:"title"`
	CreatedAt      int64  `json:"createdAt"`
}

// ConversationClosedData 是 conversation.closed 事件的载荷
type ConversationClosedData struct {
	ConversationID int64 `json:"conversationId"`
	UserID         int64 `json:"userId"`
	ClosedAt       int64 `json:"closedAt"`
}

// EventPublisher 是事件发布接口
//
// 实现：
//   - InMemoryEventPublisher：测试用，存于 slice
//   - KafkaEventPublisher：生产用，发到 Kafka
type EventPublisher interface {
	// Publish 发布事件到指定 topic
	Publish(ctx context.Context, topic string, e *Event) error
	// Close 关闭（flush 缓冲等）
	Close() error
}

// =====================================================
// InMemoryEventPublisher（测试替身）
// =====================================================

// InMemoryEventPublisher 把事件存到 slice，便于测试断言
type InMemoryEventPublisher struct {
	mu     sync.Mutex
	events map[string][]*Event // key = topic
}

// NewInMemoryEventPublisher 构造空 publisher
func NewInMemoryEventPublisher() *InMemoryEventPublisher {
	return &InMemoryEventPublisher{events: make(map[string][]*Event)}
}

func (p *InMemoryEventPublisher) Publish(ctx context.Context, topic string, e *Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events[topic] = append(p.events[topic], e)
	return nil
}

func (p *InMemoryEventPublisher) Close() error { return nil }

// Events 取已发布的事件（仅测试用）
//
// Stage 26-A 暴露 bug #7 修复：原 copy() 仅 shallow-copy slice header，
// 外部修改 Event 字段（含 Data any）会污染内部。
//
// 本实现：每条 Event 拷贝一份新指针 + Data field（如是 []byte 深拷贝）。
// 注：Data 是 any 类型，若 Data 内部是 []byte，本实现也一并深拷贝；
// 若是 map[string]any，需要业务方按需序列化（避免 string(key)/interface{} 双池问题）。
func (p *InMemoryEventPublisher) Events(topic string) []*Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	src := p.events[topic]
	out := make([]*Event, len(src))
	for i, e := range src {
		copy := *e // shallow-copy 整个 struct
		// 若 Data 是 []byte，深拷贝；其它类型维持原值引用
		if b, ok := e.Data.([]byte); ok {
			copy.Data = append([]byte(nil), b...)
		}
		out[i] = &copy
	}
	return out
}