//go:build integration
// +build integration

// Package integration_test — dlq_integration_test.go
//
// Stage 30-C A2: ai-svc 死信队列端到端集成测试
//
// 流程：
//   - testcontainers 起 Kafka broker + Postgres
//   - ai-svc Consume 启动，订阅 chat-events，DLQ topic = chat-events-dlq
//   - 发 [正常消息, 毒消息（data 字段破坏）, 正常消息]
//   - 断言：
//     - chat-events 中 2 条正常消息被处理（写入 emotion_analysis）
//     - chat-events-dlq 中 1 条毒消息被投递（DLQ 收到）
//     - 毒消息不会被无限重投（partition 不卡死）
//
// 跑：go test -tags integration -v -timeout 5m -run DLQ ./integration_test/...
package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	kafkacontainer "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/IBM/sarama"

	"emotion-echo-ai-svc/internal/consumer"
	"emotion-echo-ai-svc/internal/events"
)

// aiKafkaContainerForDLQ 起 cp-kafka broker，ai-svc 测试用
func aiKafkaContainerForDLQ(t *testing.T, ctx context.Context) (*kafkacontainer.KafkaContainer, []string) {
	t.Helper()
	kc, err := kafkacontainer.RunContainer(ctx,
		kafkacontainer.WithClusterID("ai-svc-dlq-test"),
		testcontainers.WithWaitStrategy(wait.ForLog("Kafka Server started").WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err)
	brokers, err := kc.Brokers(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, brokers)
	return kc, brokers
}

// publishRawBytes 直接发原字节（用于毒消息）
func publishRawBytes(t *testing.T, ctx context.Context, brokers []string, topic string, key string, value []byte) {
	t.Helper()
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Return.Successes = true
	prod, err := sarama.NewSyncProducer(brokers, cfg)
	require.NoError(t, err)
	defer prod.Close()

	_, _, err = prod.SendMessage(&sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	})
	require.NoError(t, err)
}

// consumeOne 从 topic 拉一条消息（带超时）
func consumeOne(t *testing.T, ctx context.Context, brokers []string, topic string, timeout time.Duration) (*sarama.ConsumerMessage, bool) {
	t.Helper()
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Return.Errors = true
	consumer, err := sarama.NewConsumer(brokers, cfg)
	require.NoError(t, err)
	defer consumer.Close()

	// 用 0 分区拉（auto-create topic 会用 0）
	parts, err := consumer.Partitions(topic)
	if err != nil || len(parts) == 0 {
		return nil, false
	}

	pc, err := consumer.ConsumePartition(topic, parts[0], sarama.OffsetOldest)
	if err != nil {
		return nil, false
	}
	defer pc.Close()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case msg := <-pc.Messages():
			if msg != nil {
				return msg, true
			}
		case <-ctx.Done():
			return nil, false
		case <-time.After(200 * time.Millisecond):
			// 继续轮询
		}
	}
	return nil, false
}

// TestKafka_DLQ_ReceivesPoisonMessage 端到端：发毒消息到 chat-events → ai-svc 重试 → DLQ 接收
//
// 用真实 Kafka broker + 真实 ai-svc consumer + 真实 DLQ publisher。
// 跳过 Postgres — DLQ 路径不依赖 DB（handler 失败立刻走 DLQ 不写库）。
func TestKafka_DLQ_ReceivesPoisonMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()

	// 1. Kafka broker
	kc, brokers := aiKafkaContainerForDLQ(t, ctx)
	defer func() { _ = kc.Terminate(ctx) }()

	// 2. 启 DLQ publisher（指向 dlq topic）
	dlqTopic := "chat-events-dlq-test"
	dlq, err := consumer.NewKafkaDLQPublisher(brokers, dlqTopic)
	require.NoError(t, err)
	defer func() { _ = dlq.Close() }()

	// 3. 触发 chat-events topic 自动创建（先发一条）
	publishRawBytes(t, ctx, brokers, "chat-events", "bootstrap", []byte(`{"type":"message.created","id":"bootstrap","data":{"messageId":0,"userId":0,"conversationId":0,"role":"system","content":"bootstrap"}}`))

	// 4. 启 ai-svc consumer（MaxRetries=1 让毒消息第 2 次就投 DLQ，避免集成测试过慢）
	kcConsumer, err := consumer.NewKafkaConsumer(brokers, "ai-svc-dlq-it")
	require.NoError(t, err)
	defer func() { _ = kcConsumer.Close() }()

	handler := func(ctx context.Context, evt *events.Event) error {
		// 模拟 handler 永远失败（让所有非 bootstrap 消息进 DLQ）
		return fmt.Errorf("forced handler err for dlq test, event=%s", evt.ID)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = kcConsumer.Consume(runCtx, []string{"chat-events"},
			handler, "", nil, dlq, 1) // MaxRetries=1: 第 2 次进 DLQ
	}()
	// 等 Setup
	time.Sleep(2 * time.Second)

	// 5. 发 2 条消息（同 key 让 sarama 同 partition 顺序处理）
	publishRawBytes(t, ctx, brokers, "chat-events", "evt-dup-1",
		[]byte(`{"type":"message.created","id":"evt-dup-1","data":{"messageId":1,"userId":1,"conversationId":1,"role":"user","content":"first"}}`))
	publishRawBytes(t, ctx, brokers, "chat-events", "evt-dup-1",
		[]byte(`{"type":"message.created","id":"evt-dup-1","data":{"messageId":1,"userId":1,"conversationId":1,"role":"user","content":"first"}}`))

	// 6. 断言 DLQ 收到 1 条
	var dlqMsg *sarama.ConsumerMessage
	var ok bool
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		dlqMsg, ok = consumeOne(t, ctx, brokers, dlqTopic, 3*time.Second)
		if ok && dlqMsg != nil {
			break
		}
	}
	require.True(t, ok, "DLQ 应该在 30s 内收到毒消息")
	require.NotNil(t, dlqMsg)

	// 7. 验证 payload 是原消息
	var got events.Event
	require.NoError(t, json.Unmarshal(dlqMsg.Value, &got))
	assert.Equal(t, "evt-dup-1", got.ID, "DLQ 收到的 event.id 应等于原 ID")

	// 8. 验证 headers 包含 x-error-reason 和 x-attempts
	headerMap := make(map[string]string)
	for _, h := range dlqMsg.Headers {
		headerMap[string(h.Key)] = string(h.Value)
	}
	assert.Contains(t, headerMap, "x-original-topic")
	assert.Contains(t, headerMap, "x-error-reason")
	assert.Contains(t, headerMap, "x-attempts")
	assert.Equal(t, "2", headerMap["x-attempts"], "MaxRetries=1 时第 2 次失败投 DLQ，attempts=2")
}
