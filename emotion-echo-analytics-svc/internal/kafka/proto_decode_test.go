// Package kafka — proto_decode_test.go
//
// Stage 73 RED：analytics consumer 兼容解析 Protobuf 事件（§1.5 双写窗口）。
//
// 契约（DecodeChatEvent）：
//   - header content-type=application/x-protobuf → proto.Unmarshal
//   - 无 header 且首字节 != '{' → 嗅探为 Protobuf
//   - 其余 → JSON（旧消息迁移窗口兼容）
//   - 解析结果为本地 events.Event，Data 为具体 Data struct（handleOne 无感）
package kafka

import (
	"testing"
	"time"

	chatevents "github.com/emotion-echo/shared/pkg/chatevents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"emotion-echo-analytics-svc/internal/events"
)

func protoEnvelope(t *testing.T) []byte {
	t.Helper()
	env := &chatevents.ChatEventEnvelope{
		Id:            "evt-p1",
		Type:          "message.created",
		Source:        "chat-svc",
		TimeUnixMilli: time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC).UnixMilli(),
		Data: &chatevents.ChatEventEnvelope_MessageCreated{
			MessageCreated: &chatevents.MessageCreatedData{
				MessageId: 21, ConversationId: 7, UserId: 2,
				Role: "user", Content: "有点焦虑", CreatedAt: 1,
			},
		},
	}
	raw, err := proto.Marshal(env)
	require.NoError(t, err)
	return raw
}

func TestDecodeChatEvent_ProtobufWithHeader(t *testing.T) {
	raw := protoEnvelope(t)
	ev, err := DecodeChatEvent(raw, map[string]string{"content-type": "application/x-protobuf"})
	require.NoError(t, err)
	assert.Equal(t, "evt-p1", ev.ID)
	assert.Equal(t, "message.created", ev.Type)
	mc, ok := ev.Data.(events.MessageCreatedData)
	require.True(t, ok, "Data 应为本地 MessageCreatedData，实际 %T", ev.Data)
	assert.Equal(t, int64(21), mc.MessageID)
	assert.Equal(t, "有点焦虑", mc.Content)
}

func TestDecodeChatEvent_ProtobufSniffedWithoutHeader(t *testing.T) {
	raw := protoEnvelope(t)
	ev, err := DecodeChatEvent(raw, nil)
	require.NoError(t, err, "无 header 时按首字节嗅探为 Protobuf")
	assert.Equal(t, "evt-p1", ev.ID)
}

func TestDecodeChatEvent_LegacyJSONFallback(t *testing.T) {
	legacy := `{"id":"evt-j1","type":"message.created","source":"chat-svc",
"time":"2026-09-12T09:00:00Z","data":{"messageId":31,"conversationId":8,"userId":1,"role":"user","content":"hi"}}`
	ev, err := DecodeChatEvent([]byte(legacy), nil)
	require.NoError(t, err, "旧 JSON 消息在迁移窗口内必须继续可消费")
	assert.Equal(t, "evt-j1", ev.ID)
	shape, err := extractDataShape(ev.Data)
	require.NoError(t, err)
	assert.Equal(t, int64(31), shape.MessageID)
}

func TestDecodeChatEvent_UnknownOneof(t *testing.T) {
	env := &chatevents.ChatEventEnvelope{Id: "evt-e", Type: "message.created"}
	raw, err := proto.Marshal(env)
	require.NoError(t, err)
	_, err = DecodeChatEvent(raw, map[string]string{"content-type": "application/x-protobuf"})
	require.Error(t, err, "type 声明 message.created 但 oneof 缺失应报错")
}
