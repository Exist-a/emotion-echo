// Package events — proto_marshal_relay_test.go
//
// Stage 73 e2e 暴露 bug（2026-09-12 docker 实测）：
// outbox relay 的 publishOne 把 JSONB payload json.Unmarshal 进 events.Event，
// Data 落成 map[string]interface{}，MarshalChatEvent 报
// "unsupported Data type map[string]interface {}" → outbox 行重试 100 次后 dead。
//
// 修复契约：UnmarshalChatEventJSON 按 Type 把 data 反序列化到具体 Data struct，
// relay 用它替代裸 json.Unmarshal。
package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalChatEventJSON_TypedData(t *testing.T) {
	payloads := map[string]string{
		EventTypeMessageCreated: `{"id":"e1","type":"message.created","source":"chat-svc",
"time":"2026-09-12T00:20:20Z","data":{"messageId":16,"conversationId":15,"userId":1,"role":"user","content":"hi"}}`,
		EventTypeConversationCreated: `{"id":"e2","type":"conversation.created","source":"chat-svc",
"time":"2026-09-12T00:20:20Z","data":{"conversationId":15,"userId":1,"title":"新会话"}}`,
		EventTypeConversationClosed: `{"id":"e3","type":"conversation.closed","source":"chat-svc",
"time":"2026-09-12T00:20:20Z","data":{"conversationId":15,"userId":1,"closedAt":1789172420}}`,
	}
	for typ, payload := range payloads {
		ev, err := UnmarshalChatEventJSON([]byte(payload))
		require.NoError(t, err, "type=%s", typ)
		require.Equal(t, typ, ev.Type)
		switch typ {
		case EventTypeMessageCreated:
			_, ok := ev.Data.(MessageCreatedData)
			require.True(t, ok, "message.created Data 应为 MessageCreatedData，实际 %T", ev.Data)
		case EventTypeConversationCreated:
			_, ok := ev.Data.(ConversationCreatedData)
			require.True(t, ok, "conversation.created Data 应为 ConversationCreatedData，实际 %T", ev.Data)
		case EventTypeConversationClosed:
			_, ok := ev.Data.(ConversationClosedData)
			require.True(t, ok, "conversation.closed Data 应为 ConversationClosedData，实际 %T", ev.Data)
		}
		// 端到端断言：typed Data 必须能过 MarshalChatEvent（relay → Kafka 路径）
		_, err = MarshalChatEvent(ev)
		require.NoError(t, err, "type=%s typed Data 应可 Protobuf 序列化", typ)
	}
}

func TestUnmarshalChatEventJSON_Roundtrip(t *testing.T) {
	at := time.Date(2026, 9, 12, 0, 20, 20, 0, time.UTC)
	orig := &Event{
		ID: "e9", Type: EventTypeMessageCreated, Source: "chat-svc", Time: at,
		Data: MessageCreatedData{MessageID: 16, ConversationID: 15, UserID: 1, Role: "user", Content: "hi"},
	}
	raw, err := MarshalChatEvent(orig) // 生产路径：protobuf
	require.NoError(t, err)
	_ = raw

	// JSON 路径（outbox 存储）：typed JSON → typed Event → protobuf 可序列化
	jsonPayload := `{"id":"e9","type":"message.created","source":"chat-svc","time":"` +
		at.Format(time.RFC3339) + `","data":{"messageId":16,"conversationId":15,"userId":1,"role":"user","content":"hi"}}`
	ev, err := UnmarshalChatEventJSON([]byte(jsonPayload))
	require.NoError(t, err)
	assert.Equal(t, int64(16), ev.Data.(MessageCreatedData).MessageID)
}
