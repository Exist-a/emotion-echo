//go:build integration
// +build integration

// Package integration_test — kafka_integration_test.go
//
// Stage 30-B RED: analytics-svc Kafka consumer 端到端集成测试
// （真实 broker + 真实 Postgres）：
//   - testcontainers 起 Kafka (confluentinc/cp-kafka) + Postgres
//   - 发布 message.created 事件到 chat-events
//   - 启动 Consumer，断言 user_behavior_events 写入一行
//
// 跑：go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kafkacontainer "github.com/testcontainers/testcontainers-go/modules/kafka"

	"emotion-echo-analytics-svc/internal/events"
	"emotion-echo-analytics-svc/internal/kafka"
	"emotion-echo-analytics-svc/internal/repository"
)

// waitFor 有界轮询：直到 cond() 为真或超时（wait loop，非 time.Sleep）
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		<-ticker.C
	}
	return cond()
}

// publishChatEvent 用 sarama sync producer 发布一条 chat-events 事件
func publishChatEvent(t *testing.T, ctx context.Context, brokers []string, e *events.Event) {
	t.Helper()
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Return.Successes = true
	prod, err := sarama.NewSyncProducer(brokers, cfg)
	require.NoError(t, err)
	defer prod.Close()

	payload, err := json.Marshal(e)
	require.NoError(t, err)
	_, _, err = prod.SendMessage(&sarama.ProducerMessage{
		Topic: events.TopicChatEvents,
		Key:   sarama.StringEncoder(e.ID), // 事件 ID 作 key → 同 id 同 partition
		Value: sarama.ByteEncoder(payload),
	})
	require.NoError(t, err, "publish %s", e.Type)
}

func TestAnalyticsKafka_Consumer_MessageCreated_WritesBehaviorEvent_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()

	// 1. Kafka broker
	kc, err := kafkacontainer.RunContainer(ctx)
	require.NoError(t, err)
	defer func() { _ = kc.Terminate(ctx) }()
	brokers, err := kc.Brokers(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, brokers)

	// 2. Postgres + repo
	pgDB, cleanup := pgContainerForEvents(t, ctx)
	defer cleanup()
	repo := repository.NewPostgresEventRepo(pgDB)

	// 3. Consumer（topic 存在后启动，OffsetOldest 消费）
	consumer, err := kafka.NewConsumer(brokers, "it-analytics-consumer", events.TopicChatEvents, repo)
	require.NoError(t, err)
	defer func() { _ = consumer.Close() }()

	// 4. 先发布事件（触发 topic 自动创建）
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	publishChatEvent(t, ctx, brokers, &events.Event{
		ID:     "evt-int-1",
		Type:   events.EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   now,
		Data: events.MessageCreatedData{
			MessageID:      7,
			ConversationID: 3,
			UserID:         42,
			Role:           "user",
			Content:        "hello from integration",
			CreatedAt:      now.UnixMilli(),
		},
	})

	// 5. 启动 consumer
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = consumer.Run(runCtx)
	}()

	// 6. 轮询断言：user_behavior_events 写入一行
	ok := waitFor(t, 20*time.Second, func() bool {
		ev, err := repo.GetByID(ctx, 1)
		return err == nil && ev != nil && ev.UserID == 42
	})
	require.True(t, ok, "consumer 应在超时内把 message.created 写入 user_behavior_events")

	got, err := repo.GetByID(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(42), got.UserID)
	assert.Equal(t, "message", got.EventType)
	assert.Equal(t, "evt-int-1", got.Target, "target 应等于事件 ID")
	assert.Equal(t, "evt-int-1", got.EventID, "Stage 30-C A1: EventID 应等于事件 ID（幂等键）")
	assert.Equal(t, "chat-events", got.SessionID, "session_id 暂用 topic 名")
	assert.True(t, got.OccurredAt.Equal(now), "OccurredAt 应等于事件时间")
}

// TestAnalyticsKafka_IdempotentOnDuplicateEventID Stage 30-C A1 端到端幂等：
// 发 [evt-dup, evt-dup, sentinel evt-new] 三条 → 启 consumer → 等 sentinel 落库 →
// 断言总行数 == 2（重复的 evt-dup 因 ON CONFLICT DO NOTHING 被去重）。
//
// sentinel 是必要的"反向证据"：单看 evt-dup 一行可能假阳性（broker 还没消费完）。
// 等 sentinel 出现意味着所有消息都已被处理。
func TestAnalyticsKafka_IdempotentOnDuplicateEventID(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()

	// 1. Kafka broker
	kc, err := kafkacontainer.RunContainer(ctx)
	require.NoError(t, err)
	defer func() { _ = kc.Terminate(ctx) }()
	brokers, err := kc.Brokers(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, brokers)

	// 2. Postgres + repo（pgContainerForEvents 已 apply migration 002）
	pgDB, cleanup := pgContainerForEvents(t, ctx)
	defer cleanup()
	repo := repository.NewPostgresEventRepo(pgDB)

	// 3. 先发布 3 条事件，触发 topic 自动创建（同 ID 用同 key → 同 partition → 顺序处理）
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	publishChatEvent(t, ctx, brokers, &events.Event{
		ID: "evt-dup-1", Type: events.EventTypeMessageCreated, Source: "chat-svc", Time: now,
		Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 100, Role: "user", Content: "first"},
	})
	publishChatEvent(t, ctx, brokers, &events.Event{
		ID: "evt-dup-1", Type: events.EventTypeMessageCreated, Source: "chat-svc", Time: now,
		Data: events.MessageCreatedData{MessageID: 1, ConversationID: 1, UserID: 100, Role: "user", Content: "first"},
	})
	publishChatEvent(t, ctx, brokers, &events.Event{
		ID: "evt-sentinel-1", Type: events.EventTypeMessageCreated, Source: "chat-svc", Time: now,
		Data: events.MessageCreatedData{MessageID: 2, ConversationID: 1, UserID: 100, Role: "user", Content: "sentinel"},
	})

	// 4. 启 consumer
	consumer, err := kafka.NewConsumer(brokers, "it-analytics-idempotency", events.TopicChatEvents, repo)
	require.NoError(t, err)
	defer func() { _ = consumer.Close() }()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = consumer.Run(runCtx) }()

	// 5. 等 sentinel 落库（保证所有消息都已处理完）
	ok := waitFor(t, 30*time.Second, func() bool {
		// sentinel EventID 是 evt-sentinel-1；GetByID 不知道 ID，按 count 检查
		var count int64
		_ = pgDB.WithContext(ctx).Raw(
			`SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events`).Scan(&count).Error
		return count >= 2
	})
	require.True(t, ok, "sentinel 应在超时内落库（count >= 2）")

	// 6. 关键断言：总行数 == 2（evt-dup-1 一行 + evt-sentinel-1 一行）
	var total int64
	require.NoError(t, pgDB.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events`).Scan(&total).Error)
	assert.Equal(t, int64(2), total, "Stage 30-C A1: 重复 evt-dup-1 应被 ON CONFLICT DO NOTHING 去重")

	// 7. 验证 evt-dup-1 确实只有一行
	var dupCount int64
	require.NoError(t, pgDB.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events WHERE event_id = ?`,
		"evt-dup-1").Scan(&dupCount).Error)
	assert.Equal(t, int64(1), dupCount, "evt-dup-1 应只落一行")
}
