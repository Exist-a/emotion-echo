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
func TestMapEventToUserBehaviorRow_UnknownEventType(t *testing.T) {
	t.Parallel()
	cases := []string{"foo.bar", "", "message.deleted", "session.ended"}
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
