// Package events — proto_marshal.go
//
// Stage 73 GREEN：事件 Protobuf 序列化（kafka-reliability-gaps.md §1.5/§3.5）。
//
// 契约：
//   - MarshalChatEvent 把 Event 转成 chatevents.ChatEventEnvelope 的 Protobuf 二进制
//   - oneof data 按 Event.Type 恰好设置其一；未知 Data 类型报错（不静默丢载荷）
//   - KafkaEventPublisher.Publish 用本函数产出 message value，并写
//     ContentTypeHeaderProto header 供 consumer 识别；旧 JSON 消息由 consumer
//     的 JSON fallback 兼容（双写窗口）
package events

import (
	"encoding/json"
	"fmt"
	"time"

	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"google.golang.org/protobuf/proto"
)

// ContentTypeHeaderProto Kafka message header 的 content-type 值
const ContentTypeHeaderProto = "application/x-protobuf"

// MarshalChatEvent 把 Event 序列化为 Protobuf
func MarshalChatEvent(e *Event) ([]byte, error) {
	env := &chatevents.ChatEventEnvelope{
		Id:            e.ID,
		Type:          e.Type,
		Source:        e.Source,
		TimeUnixMilli: e.Time.UnixMilli(),
	}
	switch d := e.Data.(type) {
	case MessageCreatedData:
		env.Data = &chatevents.ChatEventEnvelope_MessageCreated{
			MessageCreated: &chatevents.MessageCreatedData{
				MessageId:      d.MessageID,
				ConversationId: d.ConversationID,
				UserId:         d.UserID,
				Role:           d.Role,
				Content:        d.Content,
				CreatedAt:      d.CreatedAt,
			},
		}
	case *MessageCreatedData:
		if d == nil {
			return nil, fmt.Errorf("events: nil MessageCreatedData (event %s)", e.ID)
		}
		return MarshalChatEvent(&Event{ID: e.ID, Type: e.Type, Source: e.Source, Time: e.Time, Data: *d})
	case ConversationCreatedData:
		env.Data = &chatevents.ChatEventEnvelope_ConversationCreated{
			ConversationCreated: &chatevents.ConversationCreatedData{
				ConversationId: d.ConversationID,
				UserId:         d.UserID,
				Title:          d.Title,
				CreatedAt:      d.CreatedAt,
			},
		}
	case *ConversationCreatedData:
		if d == nil {
			return nil, fmt.Errorf("events: nil ConversationCreatedData (event %s)", e.ID)
		}
		return MarshalChatEvent(&Event{ID: e.ID, Type: e.Type, Source: e.Source, Time: e.Time, Data: *d})
	case ConversationClosedData:
		env.Data = &chatevents.ChatEventEnvelope_ConversationClosed{
			ConversationClosed: &chatevents.ConversationClosedData{
				ConversationId: d.ConversationID,
				UserId:         d.UserID,
				ClosedAt:       d.ClosedAt,
			},
		}
	case *ConversationClosedData:
		if d == nil {
			return nil, fmt.Errorf("events: nil ConversationClosedData (event %s)", e.ID)
		}
		return MarshalChatEvent(&Event{ID: e.ID, Type: e.Type, Source: e.Source, Time: e.Time, Data: *d})
	default:
		return nil, fmt.Errorf("events: unsupported Data type %T for event %s (type=%s)", e.Data, e.ID, e.Type)
	}
	return proto.Marshal(env)
}

// UnmarshalChatEventJSON 把 outbox JSONB payload 反序列化为 typed Event
//
// Stage 73 e2e bug 修复：relay 直接 json.Unmarshal 会让 Data 落成
// map[string]interface{}，MarshalChatEvent 拒绝 map → outbox 行重试至 dead。
// 本函数按 Type 把 data 反序列化到具体 Data struct，relay 必须使用本函数。
func UnmarshalChatEventJSON(payload []byte) (*Event, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return nil, fmt.Errorf("events: unmarshal event probe: %w", err)
	}
	switch probe.Type {
	case EventTypeMessageCreated:
		var e struct {
			ID     string             `json:"id"`
			Source string             `json:"source"`
			Time   time.Time          `json:"time"`
			Data   MessageCreatedData `json:"data"`
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("events: unmarshal %s: %w", probe.Type, err)
		}
		return &Event{ID: e.ID, Type: probe.Type, Source: e.Source, Time: e.Time, Data: e.Data}, nil
	case EventTypeConversationCreated:
		var e struct {
			ID     string                   `json:"id"`
			Source string                   `json:"source"`
			Time   time.Time                `json:"time"`
			Data   ConversationCreatedData  `json:"data"`
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("events: unmarshal %s: %w", probe.Type, err)
		}
		return &Event{ID: e.ID, Type: probe.Type, Source: e.Source, Time: e.Time, Data: e.Data}, nil
	case EventTypeConversationClosed:
		var e struct {
			ID     string                `json:"id"`
			Source string                `json:"source"`
			Time   time.Time             `json:"time"`
			Data   ConversationClosedData `json:"data"`
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("events: unmarshal %s: %w", probe.Type, err)
		}
		return &Event{ID: e.ID, Type: probe.Type, Source: e.Source, Time: e.Time, Data: e.Data}, nil
	default:
		return nil, fmt.Errorf("events: unknown event type %q", probe.Type)
	}
}
