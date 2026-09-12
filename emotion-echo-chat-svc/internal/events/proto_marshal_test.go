// Package events — proto_marshal_test.go
//
// Stage 73 RED：KafkaEventPublisher 改 Protobuf 序列化（kafka-reliability-gaps.md §1.5）。
//
// 契约：
//   - MarshalChatEvent 产出 chatevents.ChatEventEnvelope 的 Protobuf 二进制
//   - oneof data 按 Event.Type 恰好设置其一
//   - time → time_unix_milli（unix 毫秒）
//   - ContentTypeHeaderProto 常量 = "application/x-protobuf"（producer 写 Kafka header）
package events

import (
	"testing"
	"time"

	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMarshalChatEvent_MessageCreated(t *testing.T) {
	at := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	e := &Event{
		ID:     "evt-1",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   at,
		Data: MessageCreatedData{
			MessageID: 11, ConversationID: 5, UserID: 1,
			Role: "user", Content: "今天有点累", CreatedAt: at.UnixMilli(),
		},
	}
	raw, err := MarshalChatEvent(e)
	require.NoError(t, err)

	var env chatevents.ChatEventEnvelope
	require.NoError(t, proto.Unmarshal(raw, &env))
	assert.Equal(t, "evt-1", env.Id)
	assert.Equal(t, "message.created", env.Type)
	assert.Equal(t, "chat-svc", env.Source)
	assert.Equal(t, at.UnixMilli(), env.TimeUnixMilli)
	mc := env.GetMessageCreated()
	require.NotNil(t, mc, "oneof 应设置 message_created")
	assert.Equal(t, int64(11), mc.MessageId)
	assert.Equal(t, int64(5), mc.ConversationId)
	assert.Equal(t, int64(1), mc.UserId)
	assert.Equal(t, "user", mc.Role)
	assert.Equal(t, "今天有点累", mc.Content)
}

func TestMarshalChatEvent_ConversationCreatedAndClosed(t *testing.T) {
	at := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)

	created := &Event{
		ID: "evt-2", Type: EventTypeConversationCreated, Source: "chat-svc", Time: at,
		Data: ConversationCreatedData{ConversationID: 5, UserID: 1, Title: "咨询", CreatedAt: at.UnixMilli()},
	}
	raw, err := MarshalChatEvent(created)
	require.NoError(t, err)
	var env1 chatevents.ChatEventEnvelope
	require.NoError(t, proto.Unmarshal(raw, &env1))
	require.NotNil(t, env1.GetConversationCreated())
	assert.Equal(t, int64(5), env1.GetConversationCreated().ConversationId)
	assert.Equal(t, "咨询", env1.GetConversationCreated().Title)

	closed := &Event{
		ID: "evt-3", Type: EventTypeConversationClosed, Source: "chat-svc", Time: at,
		Data: ConversationClosedData{ConversationID: 5, UserID: 1, ClosedAt: at.UnixMilli()},
	}
	raw, err = MarshalChatEvent(closed)
	require.NoError(t, err)
	var env2 chatevents.ChatEventEnvelope
	require.NoError(t, proto.Unmarshal(raw, &env2))
	require.NotNil(t, env2.GetConversationClosed())
	assert.Equal(t, int64(5), env2.GetConversationClosed().ConversationId)
}

func TestMarshalChatEvent_UnknownDataType(t *testing.T) {
	e := &Event{ID: "evt-4", Type: "unknown.type", Data: map[string]any{"x": 1}}
	_, err := MarshalChatEvent(e)
	require.Error(t, err, "未知 Data 类型应报错而非静默丢载荷")
}

func TestMarshalChatEvent_ProtobufBinaryNotJSON(t *testing.T) {
	e := &Event{
		ID: "evt-5", Type: EventTypeMessageCreated, Source: "chat-svc",
		Time: time.Now(),
		Data: MessageCreatedData{MessageID: 1, ConversationID: 2, UserID: 3},
	}
	raw, err := MarshalChatEvent(e)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	assert.NotEqual(t, byte('{'), raw[0], "Protobuf 二进制不应以 '{' 开头（JSON 嗅探兼容依据）")
}
