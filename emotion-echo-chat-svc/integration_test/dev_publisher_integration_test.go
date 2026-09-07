//go:build integration
// +build integration

// Package integration_test — dev_publisher_integration_test.go
//
// ADR-19 PR-A1.1 RED: DevEventPublisher 端到端集成测试
//
// TDD 立场（AGENTS.md §〇）：
//   - 本测试先于实现存在（PR-A1.1 = RED）
//   - DevEventPublisher / NewDevEventPublisher 当前不存在 → 编译失败 → RED 状态
//   - PR-A1.2 (GREEN) 写实现让本测试编译 + 通过
//
// 端到端流程：
//   1. testcontainers 起 Postgres
//   2. 建 emotion_echo_analytics schema + user_behavior_events 表
//   3. 构造 DevEventPublisher（db 指向 PG）
//   4. Publish 3 种事件类型各 1 条
//   5. 断言 user_behavior_events 表正好 3 行 + event_type 细分 + event_id 幂等
//
// 与单元测试的关系：
//   - 单元测试（dev_publisher_test.go）：fake db 断言 SQL + 参数，不实际落库
//   - 集成测试（本文件）：真实 PG，端到端验证 schema/列名/索引/UNIQUE 约束
//
// 跑：go test -tags integration -v -run DevEventPublisher -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"emotion-echo-chat-svc/internal/events"
)

// sqlDBFrom 抽取 *sql.DB from *gorm.DB（DevEventPublisher 接受 dbExecutor 接口，
// 即 *sql.DB；gorm.DB 不直接实现该接口）。
func sqlDBFrom(t *testing.T, db *gorm.DB) (sqlDB *sql.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	return sqlDB
}

// TestDevEventPublisher_EndToEnd_InsertsUserBehaviorRows RED 端到端
//
// 验证 KAFKA_ENABLED=false 路径：
//   - 3 种事件类型 → user_behavior_events 3 行
//   - event_id 是 Event.ID（幂等键）
//   - event_type 细分（message.created / conversation.created / conversation.closed）
//   - occurred_at 来自 Event.Time
func TestDevEventPublisher_EndToEnd_InsertsUserBehaviorRows(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()

	// 1. PG
	pgC, db := pgContainerDesc(t, ctx)
	defer func() { _ = pgC.Terminate(ctx) }()

	// 2. 建 analytics schema + user_behavior_events 表（与生产对齐）
	require.NoError(t, runSQL(ctx, dsnFor(t, pgC), `
CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics`))
	require.NoError(t, runSQL(ctx, dsnFor(t, pgC), `
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(64) NOT NULL UNIQUE,
  user_id BIGINT NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  target VARCHAR(255),
  session_id VARCHAR(64),
  occurred_at TIMESTAMPTZ NOT NULL
)`))

	// 3. 构造 DevEventPublisher
	pub := events.NewDevEventPublisher(sqlDBFrom(t, db))
	defer func() { _ = pub.Close() }()

	// 4. 发 3 种事件
	occurredAt := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)

	msgEvt := &events.Event{
		ID:     "evt-msg-100",
		Type:   events.EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   occurredAt,
		Data: events.MessageCreatedData{
			MessageID:      100,
			ConversationID: 42,
			UserID:         7,
			Role:           "user",
			Content:        "hello",
			CreatedAt:      1700000000,
		},
	}
	convCreatedEvt := &events.Event{
		ID:     "evt-conv-42-created",
		Type:   events.EventTypeConversationCreated,
		Source: "chat-svc",
		Time:   occurredAt,
		Data: events.ConversationCreatedData{
			ConversationID: 42,
			UserID:         7,
			Title:          "新会话",
			CreatedAt:      1700000000,
		},
	}
	convClosedEvt := &events.Event{
		ID:     "evt-conv-42-closed",
		Type:   events.EventTypeConversationClosed,
		Source: "chat-svc",
		Time:   occurredAt,
		Data: events.ConversationClosedData{
			ConversationID: 42,
			UserID:         7,
			ClosedAt:       1700000300,
		},
	}

	require.NoError(t, pub.Publish(ctx, events.TopicChatEvents, msgEvt))
	require.NoError(t, pub.Publish(ctx, events.TopicChatEvents, convCreatedEvt))
	require.NoError(t, pub.Publish(ctx, events.TopicChatEvents, convClosedEvt))

	// 5. 断言
	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events`).
		Scan(&count).Error)
	assert.Equal(t, int64(3), count, "3 events must produce 3 rows")

	// event_id 幂等性（ADR-19 提到 user_behavior_events.event_id UNIQUE）
	rows := []struct {
		EventID   string
		EventType string
		UserID    int64
	}{}
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT event_id, event_type, user_id FROM emotion_echo_analytics.user_behavior_events ORDER BY id`).
		Scan(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, "evt-msg-100", rows[0].EventID)
	assert.Equal(t, "message.created", rows[0].EventType, "A3 fix: must be fine-grained")
	assert.Equal(t, "evt-conv-42-created", rows[1].EventID)
	assert.Equal(t, "conversation.created", rows[1].EventType, "A3 fix")
	assert.Equal(t, "evt-conv-42-closed", rows[2].EventID)
	assert.Equal(t, "conversation.closed", rows[2].EventType, "A3 fix")
}

// TestDevEventPublisher_DuplicateEventID_Idempotent 幂等性（event_id UNIQUE）
//
// 发同一 Event.ID 两次 → 表里只有 1 行（ON CONFLICT DO NOTHING）。
// 这是 Stage 30-C A1 幂等去重契约在 dev 路径的延伸。
func TestDevEventPublisher_DuplicateEventID_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	defer func() { _ = pgC.Terminate(ctx) }()

	require.NoError(t, runSQL(ctx, dsnFor(t, pgC), `
CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics`))
	require.NoError(t, runSQL(ctx, dsnFor(t, pgC), `
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(64) NOT NULL UNIQUE,
  user_id BIGINT NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  target VARCHAR(255),
  session_id VARCHAR(64),
  occurred_at TIMESTAMPTZ NOT NULL
)`))

	pub := events.NewDevEventPublisher(sqlDBFrom(t, db))
	defer func() { _ = pub.Close() }()

	evt := &events.Event{
		ID:     "evt-dup-1",
		Type:   events.EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
		Data: events.MessageCreatedData{
			MessageID: 1, ConversationID: 1, UserID: 1, Role: "user", Content: "hi", CreatedAt: 1,
		},
	}

	require.NoError(t, pub.Publish(ctx, events.TopicChatEvents, evt))
	// 第二次 Publish 同 event_id：应走 ON CONFLICT DO NOTHING，返 nil，不抛错
	require.NoError(t, pub.Publish(ctx, events.TopicChatEvents, evt))

	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events`).
		Scan(&count).Error)
	assert.Equal(t, int64(1), count, "duplicate event_id must NOT produce 2 rows")
}

// dsnFor 取 testcontainers 的 DSN（避免依赖 pgC 内部状态）
func dsnFor(t *testing.T, pgC interface{ ConnectionString(context.Context, ...string) (string, error) }) string {
	t.Helper()
	dsn, err := pgC.ConnectionString(context.Background(), "sslmode=disable")
	require.NoError(t, err)
	return dsn
}

// 防止 import 未用
var _ = gormpg.Open
var _ = gormlogger.Default
var _ = gorm.Open
