package eventrow

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedOccurred 测试用固定时间
var fixedOccurred = time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

// TestMapEventToUserBehaviorRow_MessageCreated happy path
func TestMapEventToUserBehaviorRow_MessageCreated(t *testing.T) {
	t.Parallel()
	row, err := MapEventToUserBehaviorRow(
		"evt-msg-1",
		EventTypeMessageCreated,
		DataShape{MessageID: 100, ConversationID: 42, UserID: 7},
		fixedOccurred,
	)
	require.NoError(t, err)
	assert.Equal(t, "evt-msg-1", row.EventID)
	assert.Equal(t, int64(7), row.UserID)
	assert.Equal(t, EventTypeMessageCreated, row.EventType)
	assert.Equal(t, "msg:100", row.Target)
	assert.Equal(t, "conv:42", row.SessionID)
	assert.Equal(t, fixedOccurred, row.OccurredAt)
}

// TestMapEventToUserBehaviorRow_ConversationCreated happy path
func TestMapEventToUserBehaviorRow_ConversationCreated(t *testing.T) {
	t.Parallel()
	row, err := MapEventToUserBehaviorRow(
		"evt-conv-c-1",
		EventTypeConversationCreated,
		DataShape{ConversationID: 42, UserID: 7},
		fixedOccurred,
	)
	require.NoError(t, err)
	assert.Equal(t, "evt-conv-c-1", row.EventID)
	assert.Equal(t, EventTypeConversationCreated, row.EventType)
	assert.Equal(t, "conv:42", row.Target)
	assert.Equal(t, "conv:42", row.SessionID)
}

// TestMapEventToUserBehaviorRow_ConversationClosed happy path
func TestMapEventToUserBehaviorRow_ConversationClosed(t *testing.T) {
	t.Parallel()
	row, err := MapEventToUserBehaviorRow(
		"evt-conv-x-1",
		EventTypeConversationClosed,
		DataShape{ConversationID: 42, UserID: 7},
		fixedOccurred,
	)
	require.NoError(t, err)
	assert.Equal(t, EventTypeConversationClosed, row.EventType)
}

// TestMapEventToUserBehaviorRow_EmptyEventID 边界
func TestMapEventToUserBehaviorRow_EmptyEventID(t *testing.T) {
	t.Parallel()
	_, err := MapEventToUserBehaviorRow(
		"",
		EventTypeMessageCreated,
		DataShape{MessageID: 1, ConversationID: 1, UserID: 1},
		fixedOccurred,
	)
	require.Error(t, err)
}

// TestMapEventToUserBehaviorRow_ZeroConversationID 边界（3 类型都需要）
func TestMapEventToUserBehaviorRow_ZeroConversationID(t *testing.T) {
	t.Parallel()
	cases := []string{EventTypeMessageCreated, EventTypeConversationCreated, EventTypeConversationClosed}
	for _, et := range cases {
		et := et
		t.Run(et, func(t *testing.T) {
			t.Parallel()
			_, err := MapEventToUserBehaviorRow("e1", et, DataShape{UserID: 1}, fixedOccurred)
			require.Error(t, err)
		})
	}
}

// TestMapEventToUserBehaviorRow_MessageCreated_MissingMessageID 边界
func TestMapEventToUserBehaviorRow_MessageCreated_MissingMessageID(t *testing.T) {
	t.Parallel()
	_, err := MapEventToUserBehaviorRow(
		"e1",
		EventTypeMessageCreated,
		DataShape{ConversationID: 1, UserID: 1 /* MessageID missing */},
		fixedOccurred,
	)
	require.Error(t, err)
}

// TestMapEventToUserBehaviorRow_UnknownEventType 边界
//
// Sprint A 收口（PR-A1.3 v2）后,classifyEventType 用前缀匹配:
//   - "message" / "message.*"      → message 类(只要 MessageID>0 就 OK)
//   - "conversation*"              → conversation 类
//   - 其他                         → ErrUnknownEventType
func TestMapEventToUserBehaviorRow_UnknownEventType(t *testing.T) {
	t.Parallel()
	cases := []string{"foo.bar", "", "session.ended", "unknown.event"}
	for _, et := range cases {
		et := et
		t.Run(et, func(t *testing.T) {
			t.Parallel()
			_, err := MapEventToUserBehaviorRow("e1", et, DataShape{ConversationID: 1}, fixedOccurred)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnknownEventType),
				"unknown event type must return ErrUnknownEventType (got %v)", err)
		})
	}
}

// TestMapEventToUserBehaviorRow_ClassifyBothNamingStyles 验证 PR-A1.3 v2 收口
//
// 两套命名风格（chat-svc 带点 vs analytics-svc normalize 后）都能正确分类:
func TestMapEventToUserBehaviorRow_ClassifyBothNamingStyles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		eventType string
		wantClass eventClass
	}{
		// message 类
		{"message", eventClassMessage},
		{"message.created", eventClassMessage},
		// conversation 类(analytics-svc normalize 后)
		{"conversation_created", eventClassConversation},
		{"conversation_closed", eventClassConversation},
		// conversation 类(chat-svc 原值)
		{"conversation.created", eventClassConversation},
		{"conversation.closed", eventClassConversation},
		// unknown
		{"", eventClassUnknown},
		{"foo", eventClassUnknown},
		{"unknown.event", eventClassUnknown},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.eventType, func(t *testing.T) {
			t.Parallel()
			got := classifyEventType(tc.eventType)
			assert.Equal(t, tc.wantClass, got, "classify(%q) mismatch", tc.eventType)
		})
	}
}

// TestMapEventToUserBehaviorRow_BothNamingStyles_ProducesSameTargetFormat
//
// 关键收口验证: 不管 eventType 是带点 ("message.created") 还是 normalize 后
// ("message"),target / session_id 落库格式必须一致 (msg:N / conv:N)。
// 这是 ADR-19 Sprint A 收口的真正要求: "消灭两份映射"。
func TestMapEventToUserBehaviorRow_BothNamingStyles_ProducesSameTargetFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		eventType    string
		data         DataShape
		wantTarget   string
		wantSession  string
		wantUserID   int64
	}{
		{
			eventType:   "message.created", // chat-svc 原值
			data:        DataShape{MessageID: 100, ConversationID: 42, UserID: 7},
			wantTarget:  "msg:100",
			wantSession: "conv:42",
			wantUserID:  7,
		},
		{
			eventType:   "message", // analytics-svc normalize 后
			data:        DataShape{MessageID: 100, ConversationID: 42, UserID: 7},
			wantTarget:  "msg:100",
			wantSession: "conv:42",
			wantUserID:  7,
		},
		{
			eventType:   "conversation.created",
			data:        DataShape{ConversationID: 42, UserID: 7},
			wantTarget:  "conv:42",
			wantSession: "conv:42",
			wantUserID:  7,
		},
		{
			eventType:   "conversation_created", // normalize 后
			data:        DataShape{ConversationID: 42, UserID: 7},
			wantTarget:  "conv:42",
			wantSession: "conv:42",
			wantUserID:  7,
		},
		{
			eventType:   "conversation.closed",
			data:        DataShape{ConversationID: 42, UserID: 7},
			wantTarget:  "conv:42",
			wantSession: "conv:42",
			wantUserID:  7,
		},
		{
			eventType:   "conversation_closed",
			data:        DataShape{ConversationID: 42, UserID: 7},
			wantTarget:  "conv:42",
			wantSession: "conv:42",
			wantUserID:  7,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.eventType, func(t *testing.T) {
			t.Parallel()
			row, err := MapEventToUserBehaviorRow("evt-1", tc.eventType, tc.data, fixedOccurred)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTarget, row.Target,
				"target format must be identical across naming styles (eventType=%s)", tc.eventType)
			assert.Equal(t, tc.wantSession, row.SessionID,
				"session_id format must be identical across naming styles (eventType=%s)", tc.eventType)
			assert.Equal(t, tc.wantUserID, row.UserID)
			// eventType 字符串原样透传(由调用方决定落库 enum)
			assert.Equal(t, tc.eventType, row.EventType)
		})
	}
}

// TestMapEventToUserBehaviorRow_OccurredAtUTC 时区归一化
func TestMapEventToUserBehaviorRow_OccurredAtUTC(t *testing.T) {
	t.Parallel()
	beijing := time.FixedZone("CST", 8*3600)
	in := time.Date(2026, 9, 7, 18, 0, 0, 0, beijing) // 北京时间 18:00 = UTC 10:00
	row, err := MapEventToUserBehaviorRow("e1", EventTypeMessageCreated,
		DataShape{MessageID: 1, ConversationID: 1, UserID: 1}, in)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, row.OccurredAt.Location(),
		"OccurredAt must be normalized to UTC for consistent DB timestamp")
	assert.Equal(t, 10, row.OccurredAt.Hour())
}

// TestMapEventToUserBehaviorRow_TableDriven 表驱动覆盖 3 类型
func TestMapEventToUserBehaviorRow_TableDriven(t *testing.T) {
	t.Parallel()
	cases := []struct {
		eventType string
		data      DataShape
		wantET    string
		wantTgt   string
	}{
		{
			eventType: EventTypeMessageCreated,
			data:      DataShape{MessageID: 1, ConversationID: 10, UserID: 1},
			wantET:    EventTypeMessageCreated,
			wantTgt:   "msg:1",
		},
		{
			eventType: EventTypeConversationCreated,
			data:      DataShape{ConversationID: 20, UserID: 2},
			wantET:    EventTypeConversationCreated,
			wantTgt:   "conv:20",
		},
		{
			eventType: EventTypeConversationClosed,
			data:      DataShape{ConversationID: 30, UserID: 3},
			wantET:    EventTypeConversationClosed,
			wantTgt:   "conv:30",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.eventType, func(t *testing.T) {
			t.Parallel()
			row, err := MapEventToUserBehaviorRow("e1", tc.eventType, tc.data, fixedOccurred)
			require.NoError(t, err)
			assert.Equal(t, tc.wantET, row.EventType)
			assert.Equal(t, tc.wantTgt, row.Target)
			assert.Equal(t, "conv:"+itoaInt64(tc.data.ConversationID), row.SessionID)
		})
	}
}

// itoaInt64 避免 import strconv 引入额外依赖
func itoaInt64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	buf := [21]byte{}
	pos := len(buf)
	for v > 0 {
		pos--
		buf[pos] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
