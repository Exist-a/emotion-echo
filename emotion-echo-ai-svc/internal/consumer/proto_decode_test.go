// Package consumer — proto_decode_test.go
//
// Stage 73 RED：ai-svc consumer 兼容解析 Protobuf 事件（§1.5 双写窗口）。
//
// 契约（与 analytics-svc 的 DecodeChatEvent 同构）：
//   - header content-type=application/x-protobuf → proto.Unmarshal
//   - 无 header 且首字节 != '{' → 嗅探为 Protobuf
//   - 其余 → JSON（旧消息迁移窗口兼容）
package consumer

import (
	"testing"
	"time"

	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"emotion-echo-ai-svc/internal/events"
)

func TestDecodeChatEvent_ProtobufWithHeader(t *testing.T) {
	env := &chatevents.ChatEventEnvelope{
		Id:            "evt-ai1",
		Type:          "message.created",
		Source:        "chat-svc",
		TimeUnixMilli: time.Date(2026, 9, 12, 9, 30, 0, 0, time.UTC).UnixMilli(),
		Data: &chatevents.ChatEventEnvelope_MessageCreated{
			MessageCreated: &chatevents.MessageCreatedData{
				MessageId: 41, ConversationId: 9, UserId: 1,
				Role: "user", Content: "睡不太好",
			},
		},
	}
	raw, err := proto.Marshal(env)
	require.NoError(t, err)

	ev, err := DecodeChatEvent(raw, map[string]string{"content-type": "application/x-protobuf"})
	require.NoError(t, err)
	assert.Equal(t, "evt-ai1", ev.ID)
	assert.Equal(t, "message.created", ev.Type)
	mc, ok := ev.Data.(events.MessageCreatedData)
	require.True(t, ok, "Data 应为本地 MessageCreatedData，实际 %T", ev.Data)
	assert.Equal(t, int64(41), mc.MessageID)
	assert.Equal(t, "睡不太好", mc.Content)
}

func TestDecodeChatEvent_ProtobufSniffedWithoutHeader(t *testing.T) {
	env := &chatevents.ChatEventEnvelope{
		Id:   "evt-ai2",
		Type: "message.created",
		Data: &chatevents.ChatEventEnvelope_MessageCreated{
			MessageCreated: &chatevents.MessageCreatedData{MessageId: 1, ConversationId: 2, UserId: 3},
		},
	}
	raw, err := proto.Marshal(env)
	require.NoError(t, err)

	ev, err := DecodeChatEvent(raw, nil)
	require.NoError(t, err)
	assert.Equal(t, "evt-ai2", ev.ID)
}

func TestDecodeChatEvent_LegacyJSONFallback(t *testing.T) {
	legacy := `{"id":"evt-ai3","type":"message.created","source":"chat-svc",
"time":"2026-09-12T09:00:00Z","data":{"messageId":51,"conversationId":8,"userId":1,"role":"user","content":"hi"}}`
	ev, err := DecodeChatEvent([]byte(legacy), nil)
	require.NoError(t, err, "旧 JSON 消息在迁移窗口内必须继续可消费")
	assert.Equal(t, "evt-ai3", ev.ID)
}

func TestDecodeChatEvent_NonMessageTypesSkippable(t *testing.T) {
	// conversation.* 事件 ai-svc 不关心：解码不报错，Type 供 TopicFilter 过滤
	env := &chatevents.ChatEventEnvelope{
		Id:   "evt-ai4",
		Type: "conversation.closed",
		Data: &chatevents.ChatEventEnvelope_ConversationClosed{
			ConversationClosed: &chatevents.ConversationClosedData{ConversationId: 8, UserId: 1, ClosedAt: 1},
		},
	}
	raw, err := proto.Marshal(env)
	require.NoError(t, err)

	ev, err := DecodeChatEvent(raw, nil)
	require.NoError(t, err)
	assert.Equal(t, "conversation.closed", ev.Type)
}
