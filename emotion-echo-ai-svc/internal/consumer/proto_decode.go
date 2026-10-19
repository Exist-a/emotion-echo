// Package consumer — proto_decode.go
//
// Stage 73 GREEN：ai-svc consumer 兼容解析 Protobuf 事件（§1.5 双写窗口）。
// 与 analytics-svc internal/kafka/proto_decode.go 同构（各自镜像本地 events 包）。
//
// ai-svc 只消费 message.created；conversation.* 解码不报错，
// Data 为 nil，由 ConsumeClaim 的 TopicFilter 过滤跳过。
package consumer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"google.golang.org/protobuf/proto"

	"emotion-echo-ai-svc/internal/events"
)

// DecodeChatEvent 把 Kafka message value 解析为本地 events.Event
func DecodeChatEvent(raw []byte, headers map[string]string) (*events.Event, error) {
	if isProtobufPayload(raw, headers) {
		return decodeProtoChatEvent(raw)
	}
	var ev events.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, fmt.Errorf("consumer: decode event (json fallback): %w", err)
	}
	return &ev, nil
}

// isProtobufPayload 判断 payload 是否为 Protobuf 编码
func isProtobufPayload(raw []byte, headers map[string]string) bool {
	if headers != nil && headers["content-type"] == "application/x-protobuf" {
		return true
	}
	return len(raw) > 0 && raw[0] != '{'
}

// decodeProtoChatEvent Protobuf → 本地 Event
func decodeProtoChatEvent(raw []byte) (*events.Event, error) {
	var env chatevents.ChatEventEnvelope
	if err := proto.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("consumer: decode event (protobuf): %w", err)
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
	case *chatevents.ChatEventEnvelope_ConversationClosed:
		// CL-08a：记忆抽取消费组需要 conversation.closed 触发"会话收尾合并"。
		// 此前这里走 default ⇒ Data=nil ⇒ 处理器拿到零值直接 return（生产路径断路，
		// 单测因手工构造 Data 而假绿）。与 chat-svc 的 JSON 载荷字段一一对应。
		ev.Data = events.ConversationClosedData{
			ConversationID: d.ConversationClosed.GetConversationId(),
			UserID:         d.ConversationClosed.GetUserId(),
			ClosedAt:       d.ConversationClosed.GetClosedAt(),
		}
	default:
		// conversation.created：ai-svc 不消费，解码不报错，Data=nil，交由 TopicFilter 过滤
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
