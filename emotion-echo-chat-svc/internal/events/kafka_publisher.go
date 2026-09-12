// Package events 的 Kafka 生产实现
package events

import (
	"context"
	"log"

	"github.com/IBM/sarama"
)

// KafkaEventPublisher 是 EventPublisher 的 sarama 实现
//
// 复用 shared 仓里的 sarama 客户端，避免每个 svc 重复写
type KafkaEventPublisher struct {
	producer sarama.SyncProducer
}

// NewKafkaEventPublisher 用 sarama 构造 Kafka 发布器
//
// brokers：Kafka 地址列表，如 []string{"localhost:9092"}
func NewKafkaEventPublisher(brokers []string) (*KafkaEventPublisher, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll          // 强 durability
	cfg.Producer.Retry.Max = 5                              // 5 次重试
	cfg.Producer.Return.Successes = true                   // 同步等待成功
	cfg.Producer.Return.Errors = true                       // 错误回传
	cfg.Producer.Partitioner = sarama.NewHashPartitioner   // 同 key 落同 partition
	cfg.Version = sarama.V2_8_0_0                          // 兼容 Kafka 2.x/3.x

	producer, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, err
	}
	return &KafkaEventPublisher{producer: producer}, nil
}

// Publish 同步发布事件到 topic
//
// Stage 73：payload 改 Protobuf 编码（§1.5 迁移），并写 content-type header 供
// consumer 识别；旧 JSON 消息由 consumer 的 JSON fallback 兼容（双写窗口）。
// 事件 ID 作为 message key（保证同事件 id 落同 partition，便于消费者去重）。
func (p *KafkaEventPublisher) Publish(ctx context.Context, topic string, e *Event) error {
	body, err := MarshalChatEvent(e)
	if err != nil {
		return err
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(e.ID),
		Value: sarama.ByteEncoder(body),
		Headers: []sarama.RecordHeader{
			{Key: []byte("content-type"), Value: []byte(ContentTypeHeaderProto)},
		},
	}
	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		log.Printf("[kafka] publish failed: topic=%s err=%v", topic, err)
		return err
	}
	log.Printf("[kafka] published: topic=%s id=%s type=%s", topic, e.ID, e.Type)
	return nil
}

// Close 关闭 producer
func (p *KafkaEventPublisher) Close() error {
	return p.producer.Close()
}