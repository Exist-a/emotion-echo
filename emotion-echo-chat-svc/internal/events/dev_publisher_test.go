// Package events — dev_publisher_test.go
//
// ADR-19 PR-A1.1 RED: DevEventPublisher 单元测试骨架
//
// TDD 立场（AGENTS.md §〇）：
//   - 本测试文件先于实现存在（PR-A1.1 = RED）
//   - DevEventPublisher / NewDevEventPublisher 当前不存在 → 编译失败 → RED 状态
//   - PR-A1.2 (GREEN) 写实现让本文件编译通过 + 单测绿
//   - PR-A1.3 (REFACTOR) 抽 eventrow.MapEventToUserBehaviorRow 到 shared 包
//
// 目标契约（per ADR-19 §A / §C / §D）：
//   - DevEventPublisher 实现 events.EventPublisher 接口
//   - KAFKA_ENABLED=false 路径替代 nil publisher
//   - Publish 把 Event 映射到 emotion_echo_analytics.user_behavior_events 行
//     （message.created / conversation.created / conversation.closed 三类型）
//   - 同步语义：落库成功才返 nil（outbox relay 据此 MarkSent）
//   - 落库失败 → 返 error（relay MarkFailed，下次重试）
//
// 包级守卫（编译断言）：
//   - var _ EventPublisher = (*DevEventPublisher)(nil)  — 接口合规
//
// 单元测试覆盖矩阵（PR-A1.2 GREEN 阶段全部跑通）：
//
//   happy path（3 类型 × DB success）        = 3 case
//   DB error（3 类型 × ExecContext returns err）= 3 case
//   未知 event_type（UnknownType × 2）       = 2 case
//   Close nil op                           = 1 case
//   ─────────────────────────────────────────────
//   合计                                  ≥ 9 case（与 ADR-19 §验收对齐）
//
// 单元测试不做端到端 DB 验证 — 那是 dev_publisher_integration_test.go 的工作
// （testcontainers PG + 真 INSERT + 查表断言）。
package events

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDevEventPublisher_ImplementsEventPublisher 接口合规编译断言
//
// RED 状态：DevEventPublisher 类型不存在 → 编译失败。
// GREEN 后：编译通过 + 运行时类型断言验证。
func TestDevEventPublisher_ImplementsEventPublisher(t *testing.T) {
	t.Parallel()
	var _ EventPublisher = (*DevEventPublisher)(nil)
}

// TestDevEventPublisher_Close_NilOp Close 应为 no-op（接口契约）
//
// DevEventPublisher 没有后台 goroutine（与 KafkaEventPublisher 不同），
// Close 不需要 flush 任何东西。返回 nil 即可。
func TestDevEventPublisher_Close_NilOp(t *testing.T) {
	t.Parallel()
	p := NewDevEventPublisher(nil /*db*/)
	require.NotNil(t, p)
	assert.NoError(t, p.Close())
	// 重复 Close 也安全
	for i := 0; i < 3; i++ {
		assert.NoError(t, p.Close(), "Close #%d should be idempotent", i)
	}
}

// fakeDBExecutor 记录最近一次 ExecContext 调用，便于单测断言 SQL 与参数。
//
// 真实 *sql.DB 也实现 dbExecutor 接口（database/sql.DB.ExecContext 签名一致），
// 所以 DevEventPublisher 接受 dbExecutor 而非 *sql.DB 可同时满足 prod 与 test。
type fakeDBExecutor struct {
	mu       sync.Mutex
	calls    []fakeCall
	returnErr error // 设置后所有 ExecContext 都返此错误
}

type fakeCall struct {
	query string
	args  []any
}

func (f *fakeDBExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{query: query, args: args})
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return fakeResult{}, nil
}

func (f *fakeDBExecutor) lastCall() (fakeCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return fakeCall{}, false
	}
	return f.calls[len(f.calls)-1], true
}

func (f *fakeDBExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// fakeResult 实现 sql.Result 用于 fakeDBExecutor 成功路径
type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 1, nil }

// 测试 Event 构造器

func newMessageCreatedEvent() *Event {
	return &Event{
		ID:     "evt-msg-1",
		Type:   EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
		Data: MessageCreatedData{
			MessageID:      100,
			ConversationID: 42,
			UserID:         7,
			Role:           "user",
			Content:        "hello",
			CreatedAt:      1700000000,
		},
	}
}

func newConversationCreatedEvent() *Event {
	return &Event{
		ID:     "evt-conv-c-1",
		Type:   EventTypeConversationCreated,
		Source: "chat-svc",
		Time:   time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
		Data: ConversationCreatedData{
			ConversationID: 42,
			UserID:         7,
			Title:          "新会话",
			CreatedAt:      1700000000,
		},
	}
}

func newConversationClosedEvent() *Event {
	return &Event{
		ID:     "evt-conv-x-1",
		Type:   EventTypeConversationClosed,
		Source: "chat-svc",
		Time:   time.Date(2026, 9, 7, 10, 5, 0, 0, time.UTC),
		Data: ConversationClosedData{
			ConversationID: 42,
			UserID:         7,
			ClosedAt:       1700000300,
		},
	}
}

// TestDevEventPublisher_Publish_MessageCreated_HappyPath
// GREEN 后行为：
//   - Publish 调一次 ExecContext（INSERT INTO emotion_echo_analytics.user_behavior_events ...）
//   - 返 nil
//   - SQL 包含目标 schema + 表名
//   - 参数包含 event_id/event_type/target/session_id/user_id
//   - event_type = 'message.created'（细分，不合并成 'message'）
//   - target = 'msg:100' 或类似 message.id 语义（ADR-19 §D 约定）
//   - session_id = 'conv:42'（conversation.id 语义）
func TestDevEventPublisher_Publish_MessageCreated_HappyPath(t *testing.T) {
	t.Parallel()
	db := &fakeDBExecutor{}
	p := NewDevEventPublisher(db)
	defer func() { _ = p.Close() }()

	err := p.Publish(context.Background(), TopicChatEvents, newMessageCreatedEvent())
	require.NoError(t, err)

	assert.Equal(t, 1, db.callCount())
	call, ok := db.lastCall()
	require.True(t, ok)
	assert.Contains(t, call.query, "emotion_echo_analytics.user_behavior_events",
		"SQL must target the user_behavior_events table in analytics schema")
	assert.Contains(t, call.query, "INSERT",
		"SQL must be an INSERT statement")

	// 参数断言（顺序由实现决定，但每个必传）
	argsStr := joinArgs(call.args)
	assert.Contains(t, argsStr, "evt-msg-1", "event_id must be the Event.ID")
	assert.Contains(t, argsStr, "message.created", "event_type must be fine-grained, NOT merged into 'message'")
	assert.Contains(t, argsStr, "7", "user_id must come from Event.Data.UserID")
}

// TestDevEventPublisher_Publish_ConversationCreated_HappyPath
func TestDevEventPublisher_Publish_ConversationCreated_HappyPath(t *testing.T) {
	t.Parallel()
	db := &fakeDBExecutor{}
	p := NewDevEventPublisher(db)
	defer func() { _ = p.Close() }()

	err := p.Publish(context.Background(), TopicChatEvents, newConversationCreatedEvent())
	require.NoError(t, err)

	assert.Equal(t, 1, db.callCount())
	call, ok := db.lastCall()
	require.True(t, ok)
	argsStr := joinArgs(call.args)
	assert.Contains(t, argsStr, "evt-conv-c-1", "event_id must be Event.ID")
	assert.Contains(t, argsStr, "conversation.created",
		"event_type must be 'conversation.created' (A3 fix: NOT merged into 'conversation')")
	assert.Contains(t, argsStr, "7", "user_id must come from Event.Data.UserID")
}

// TestDevEventPublisher_Publish_ConversationClosed_HappyPath
func TestDevEventPublisher_Publish_ConversationClosed_HappyPath(t *testing.T) {
	t.Parallel()
	db := &fakeDBExecutor{}
	p := NewDevEventPublisher(db)
	defer func() { _ = p.Close() }()

	err := p.Publish(context.Background(), TopicChatEvents, newConversationClosedEvent())
	require.NoError(t, err)

	assert.Equal(t, 1, db.callCount())
	call, ok := db.lastCall()
	require.True(t, ok)
	argsStr := joinArgs(call.args)
	assert.Contains(t, argsStr, "evt-conv-x-1")
	assert.Contains(t, argsStr, "conversation.closed",
		"event_type must be 'conversation.closed' (A3 fix)")
	assert.Contains(t, argsStr, "7")
}

// TestDevEventPublisher_Publish_DBError_Propagates 三类型 × DB error 路径
func TestDevEventPublisher_Publish_DBError_Propagates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		evt  *Event
	}{
		{"message.created", newMessageCreatedEvent()},
		{"conversation.created", newConversationCreatedEvent()},
		{"conversation.closed", newConversationClosedEvent()},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wantErr := errors.New("simulated db failure")
			db := &fakeDBExecutor{returnErr: wantErr}
			p := NewDevEventPublisher(db)
			defer func() { _ = p.Close() }()

			err := p.Publish(context.Background(), TopicChatEvents, tc.evt)
			require.Error(t, err)
			assert.ErrorIs(t, err, wantErr,
				"DB error must propagate so outbox relay can MarkFailed + retry")
		})
	}
}

// TestDevEventPublisher_Publish_UnknownType_ReturnsError 未知事件类型
func TestDevEventPublisher_Publish_UnknownType_ReturnsError(t *testing.T) {
	t.Parallel()
	cases := []*Event{
		{ID: "x", Type: "unknown.event", Data: nil},
		{ID: "y", Type: "", Data: nil},
	}
	for i, e := range cases {
		e := e
		t.Run("case_"+e.Type, func(t *testing.T) {
			t.Parallel()
			db := &fakeDBExecutor{}
			p := NewDevEventPublisher(db)
			defer func() { _ = p.Close() }()

			err := p.Publish(context.Background(), TopicChatEvents, e)
			require.Error(t, err, "case %d: unknown event type must error", i)
			assert.Equal(t, 0, db.callCount(),
				"case %d: must NOT hit DB when event type unknown", i)
		})
	}
}

// TestDevEventPublisher_Publish_NilEvent_ReturnsError nil event 不允许
func TestDevEventPublisher_Publish_NilEvent_ReturnsError(t *testing.T) {
	t.Parallel()
	db := &fakeDBExecutor{}
	p := NewDevEventPublisher(db)
	defer func() { _ = p.Close() }()

	err := p.Publish(context.Background(), TopicChatEvents, nil)
	require.Error(t, err)
	assert.Equal(t, 0, db.callCount(), "nil event must NOT hit DB")
}

// joinArgs 把 ExecContext 的 args 拼成字符串便于 Contains 断言
func joinArgs(args []any) string {
	out := ""
	for _, a := range args {
		switch v := a.(type) {
		case string:
			out += v + "|"
		case int64:
			out += itoaInt64(v) + "|"
		case int:
			out += itoaInt(v) + "|"
		case time.Time:
			out += v.UTC().Format(time.RFC3339Nano) + "|"
		default:
			out += "<?>|"
		}
	}
	return out
}

func itoaInt(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := [20]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

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
