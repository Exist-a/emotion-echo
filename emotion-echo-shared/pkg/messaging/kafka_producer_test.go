//go:build integration

// KafkaProducer 集成测试：默认 skip，运行时通过 -tags integration 启用。
//
// 设计目的：
//   - 这些测试连接 localhost:9092 真实 Kafka
//   - 不依赖网络就不能跑，所以 build tag 隔离
//   - 单测时 `go test ./...` 自动跳过，避免 CI 默认环境失败
package messaging

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kafkaBroker 默认地址（可被 KAFKA_BROKER 环境变量覆盖）
func kafkaBroker() string {
	if v := os.Getenv("KAFKA_BROKER"); v != "" {
		return v
	}
	return "localhost:9092"
}

// uniqueTopic 每次测试一个唯一 topic，避免相互干扰
func uniqueTopic(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func TestKafkaProducer_Publish_Succeeds(t *testing.T) {
	brokers := []string{kafkaBroker()}

	prod, err := NewKafkaProducer(brokers, KafkaProducerOptions{
		ClientID: "test-producer",
	})
	require.NoError(t, err, "KafkaProducer 构造失败；检查 9092 broker 是否可达")
	defer prod.Close()

	topic := uniqueTopic("kafka-ok")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = prod.Publish(ctx, Message{
		Topic: topic,
		Value: []byte("hello kafka"),
		Headers: map[string]string{
			"trace": "red-green-refactor",
		},
	})
	require.NoError(t, err, "Publish 应当成功")
}

func TestKafkaProducer_Publish_RespectsContext(t *testing.T) {
	prod, err := NewKafkaProducer([]string{kafkaBroker()}, KafkaProducerOptions{ClientID: "test-ctx"})
	require.NoError(t, err)
	defer prod.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err = prod.Publish(ctx, Message{
		Topic: uniqueTopic("kafka-ctx"),
		Value: []byte("hi"),
	})
	require.Error(t, err, "已超时 ctx 必须返回 error")
}

func TestKafkaProducer_Publish_BrokerUnreachable(t *testing.T) {
	// 故意连一个不存在的地址
	prod, err := NewKafkaProducer(
		[]string{"localhost:1"}, // 1 端口无人监听
		KafkaProducerOptions{
			ClientID:  "test-unreachable",
			TimeoutMs: 500, // 给 500ms，不能让测试卡太久
		},
	)
	// 构造可能成功（异步），也可能失败（同步）
	if err != nil {
		t.Skipf("KafkaProducer 创建失败（预期）：%v", err)
	}
	defer prod.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = prod.Publish(ctx, Message{
		Topic: "no-broker",
		Value: []byte("hi"),
	})
	require.Error(t, err, "broker 不可达必须返回 error")
	// 不强求具体错误文字，但应是非 nil
	assert.NotNil(t, err)
}
