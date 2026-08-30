//go:build integration
// +build integration

// Package integration_test — outbox_integration_test.go
//
// Stage 30-C A3: chat-svc 事务性 Outbox 端到端集成测试
//
// 流程：
//   - testcontainers 起 Postgres + Kafka
//   - 起 emotion_echo_chat schema + conversations + outbox_events 表
//   - 用真 PostgresConversationRepo + 真 PostgresOutboxRepo
//   - 写一条 conversation（同事务）+ 写 outbox 行（同事务）
//   - 断言 outbox 行存在 + status='pending'
//   - 起 Relay + FlushOnce
//   - 断言 outbox 行 status='sent' + Kafka topic 收到事件
//
// 跑：go test -tags integration -v -run Outbox -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kafkacontainer "github.com/testcontainers/testcontainers-go/modules/kafka"

	"github.com/IBM/sarama"
	"gorm.io/gorm"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/outbox"
	"emotion-echo-chat-svc/internal/repository"
)

// TestChatOutbox_EndToEnd_RelaysToKafka Stage 30-C A3 端到端
func TestChatOutbox_EndToEnd_RelaysToKafka(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()

	// 1. PG
	pgC, db := pgContainerDesc(t, ctx)
	defer func() { _ = pgC.Terminate(ctx) }()

	require.NoError(t, db.Exec(`
CREATE TABLE IF NOT EXISTS emotion_echo_chat.outbox_events (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(64) NOT NULL UNIQUE,
  event_type VARCHAR(64) NOT NULL,
  topic VARCHAR(64) NOT NULL,
  payload JSONB NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  sent_at TIMESTAMPTZ
)`).Error)

	convRepo := repository.NewPostgresConversationRepo(db)
	outboxRepo := repository.NewPostgresOutboxRepo(db)

	// 2. Kafka
	kc, err := kafkacontainer.RunContainer(ctx,
		kafkacontainer.WithClusterID("chat-outbox-test"),
	)
	require.NoError(t, err)
	defer func() { _ = kc.Terminate(ctx) }()
	brokers, err := kc.Brokers(ctx)
	require.NoError(t, err)

	// 3. 触发 topic 自动创建
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Return.Successes = true
	prod, err := sarama.NewSyncProducer(brokers, cfg)
	require.NoError(t, err)
	_, _, err = prod.SendMessage(&sarama.ProducerMessage{
		Topic: events.TopicChatEvents,
		Key:   sarama.StringEncoder("bootstrap"),
		Value: sarama.ByteEncoder([]byte(`{"id":"bootstrap","type":"bootstrap","source":"test"}`)),
	})
	require.NoError(t, err)
	_ = prod.Close()

	// 4. 业务写入 + outbox（同事务）
	conv := &model.Conversation{UserID: 100, Title: "集成测试", Status: 1}
	evt := &events.Event{
		ID:     "evt-outbox-int-1",
		Type:   events.EventTypeConversationCreated,
		Source: "chat-svc",
		Time:   time.Now(),
		Data:   events.ConversationCreatedData{UserID: 100, Title: "集成测试", CreatedAt: time.Now().UnixMilli()},
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := convRepo.CreateConversationTx(tx, ctx, conv); err != nil {
			return err
		}
		d := evt.Data.(events.ConversationCreatedData)
		d.ConversationID = conv.ID
		evt.Data = d
		payload, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		return outboxRepo.CreateInTx(tx, &repository.OutboxEvent{
			EventID:   evt.ID,
			EventType: evt.Type,
			Topic:     events.TopicChatEvents,
			Payload:   payload,
		})
	})
	require.NoError(t, err)

	// 5. 断言：outbox 表有 1 条 pending
	var pendingCount int64
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_chat.outbox_events WHERE status = 'pending'`).Scan(&pendingCount).Error)
	require.Equal(t, int64(1), pendingCount)

	// 6. 起 Kafka publisher + Relay
	kp, err := events.NewKafkaEventPublisher(brokers)
	require.NoError(t, err)
	defer func() { _ = kp.Close() }()
	relay := outbox.NewRelay(outboxRepo, kp, 200*time.Millisecond, 100)
	require.NoError(t, relay.FlushOnce(ctx))

	// 7. 断言：outbox 表已 sent
	var sentCount int64
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM emotion_echo_chat.outbox_events WHERE status = 'sent'`).Scan(&sentCount).Error)
	assert.Equal(t, int64(1), sentCount, "FlushOnce 后应 1 条 sent")

	// 8. 断言：Kafka topic 收到 evt-outbox-int-1
	cfg2 := sarama.NewConfig()
	cfg2.Version = sarama.V2_8_0_0
	csmr, err := sarama.NewConsumer(brokers, cfg2)
	require.NoError(t, err)
	defer csmr.Close()
	parts, err := csmr.Partitions(events.TopicChatEvents)
	require.NoError(t, err)
	require.NotEmpty(t, parts)

	pc, err := csmr.ConsumePartition(events.TopicChatEvents, parts[0], sarama.OffsetOldest)
	require.NoError(t, err)
	defer pc.Close()

	deadline := time.Now().Add(15 * time.Second)
	var got *events.Event
	for time.Now().Before(deadline) {
		select {
		case msg := <-pc.Messages():
			if msg == nil {
				continue
			}
			var e events.Event
			if err := json.Unmarshal(msg.Value, &e); err != nil {
				continue
			}
			if e.ID == "evt-outbox-int-1" {
				got = &e
			}
		case <-time.After(300 * time.Millisecond):
		}
		if got != nil {
			break
		}
	}
	require.NotNil(t, got, "Kafka topic 应在 15s 内收到 evt-outbox-int-1")
	assert.Equal(t, events.EventTypeConversationCreated, got.Type)
	// Data 经 JSON 反序列化后是 map[string]interface{}（sarama 不做类型断言）
	dataMap, ok := got.Data.(map[string]interface{})
	require.True(t, ok, "Data 应是 map[string]interface{}，got %T", got.Data)
	assert.Equal(t, float64(100), dataMap["userId"], "userId 应是 100（JSON 数字 → float64）")
	assert.Equal(t, "集成测试", dataMap["title"])
}
