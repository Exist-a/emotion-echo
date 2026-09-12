// Package kafka — proto_decode.go
//
// Stage 73 GREEN：consumer 兼容解析 Protobuf 事件（kafka-reliability-gaps.md §1.5 双写窗口）。
//
// 识别顺序：
//   1. header content-type == application/x-protobuf → Protobuf
//   2. 无 header / 其他值且首字节 != '{'            → 嗅探为 Protobuf（防御旧 producer 漏 header）
//   3. 其余                                          → JSON（旧消息迁移窗口兼容）
//
// 返回的 Event.Data 是具体 Data struct（handleOne / extractDataShape 无感）。
package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"google.golang.org/protobuf/proto"

	"emotion-echo-analytics-svc/internal/events"
)

// DecodeChatEvent 把 Kafka message value 解析为本地 events.Event
func DecodeChatEvent(raw []byte, headers map[string]string) (*events.Event, error) {
	if isProtobufPayload(raw, headers) {
		return decodeProtoChatEvent(raw)
	}
	var ev events.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, fmt.Errorf("kafka-consumer: decode event (json fallback): %w", err)
	}
	return &ev, nil
}

// isProtobufPayload 判断 payload 是否为 Protobuf 编码
func isProtobufPayload(raw []byte, headers map[string]string) bool {
	if headers != nil && headers["content-type"] == "application/x-protobuf" {
		return true
	}
	// 嗅探：旧 JSON 的首字节恒为 '{'；Protobuf 二进制（field 1 string id tag=0x0A）不会是 '{'
	return len(raw) > 0 && raw[0] != '{'
}

// decodeProtoChatEvent Protobuf → 本地 Event（oneof → 具体 Data struct）
func decodeProtoChatEvent(raw []byte) (*events.Event, error) {
	var env chatevents.ChatEventEnvelope
	if err := proto.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("kafka-consumer: decode event (protobuf): %w", err)
	}
	ev := &events.Event{
		ID:     env.GetId(),
		Type:   env.GetType(),
		Source: env.GetSource(),
		Time:   time.UnixMilli(env.GetTimeUnixMilli()),
	}
	switch d := env.Data.(type) {
	case *chatevents.ChatEventEnvelope_MessageCreated:
		ev.Data = events.MessageCreatedData{
			MessageID:      d.MessageCreated.GetMessageId(),
			ConversationID: d.MessageCreated.GetConversationId(),
			UserID:         d.MessageCreated.GetUserId(),
			Role:           d.MessageCreated.GetRole(),
			Content:        d.MessageCreated.GetContent(),
			CreatedAt:      d.MessageCreated.GetCreatedAt(),
		}
	case *chatevents.ChatEventEnvelope_ConversationCreated:
		ev.Data = events.ConversationCreatedData{
			ConversationID: d.ConversationCreated.GetConversationId(),
			UserID:         d.ConversationCreated.GetUserId(),
			Title:          d.ConversationCreated.GetTitle(),
			CreatedAt:      d.ConversationCreated.GetCreatedAt(),
		}
	case *chatevents.ChatEventEnvelope_ConversationClosed:
		ev.Data = events.ConversationClosedData{
			ConversationID: d.ConversationClosed.GetConversationId(),
			UserID:         d.ConversationClosed.GetUserId(),
			ClosedAt:       d.ConversationClosed.GetClosedAt(),
		}
	default:
		return nil, fmt.Errorf("kafka-consumer: envelope %s (type=%s) missing payload oneof", env.GetId(), env.GetType())
	}
	return ev, nil
}

// saramaHeaders 把 sarama message headers 转成 map（DecodeChatEvent 入参）
func saramaHeaders(msg *sarama.ConsumerMessage) map[string]string {
	if len(msg.Headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(msg.Headers))
	for _, h := range msg.Headers {
		out[string(h.Key)] = string(h.Value)
	}
	return out
}
