// Package events 的 Kafka 生产实现
package events

import (
	"context"
	"log"

	"github.com/IBM/sarama"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

// KafkaEventPublisher 是 EventPublisher 的 sarama 实现
//
// 复用 shared 仓里的 sarama 客户端，避免每个 svc 重复写
type KafkaEventPublisher struct {
	producer sarama.SyncProducer
	// tracer Stage 92 PR-1：可选 SkyWalking tracer，注入 sw8 到 Kafka header
	// 让 ai-svc / analytics-svc consumer 通过 CreateEntrySpan 重建父 trace
	// （跨进程 trace）。nil 时降级原行为（不写 sw8 header）。
	tracer grpcinterceptor.Tracer
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

// WithTracer Stage 92 PR-1：注入 SkyWalking tracer（builder 模式）。
//
// tracer=nil 等价不调（保持向后兼容，dev / 单测场景）。
// 生产 chain 调用方：NewKafkaEventPublisher(...).WithTracer(grpcinterceptor.NewGo2SkyTracer(t))
func (p *KafkaEventPublisher) WithTracer(tracer grpcinterceptor.Tracer) *KafkaEventPublisher {
	p.tracer = tracer
	return p
}

// sw8HeaderName Stage 92 PR-1：sw8 trace header key 名。
// 复用 go2sky propagation.Header 常量，保持跨语言兼容。
const sw8HeaderName = "sw8"

// Publish 同步发布事件到 topic
//
// Stage 73：payload 改 Protobuf 编码（§1.5 迁移），并写 content-type header 供
// consumer 识别；旧 JSON 消息由 consumer 的 JSON fallback 兼容（双写窗口）。
// 事件 ID 作为 message key（保证同事件 id 落同 partition，便于消费者去重）。
//
// Stage 92 PR-1：当 p.tracer 非 nil 时，调 CreateExitSpan 把 sw8 注入 Kafka
// header（跨进程 trace）；nil tracer 时降级原行为（不破坏 dev / in-memory）。
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
	// Stage 92 PR-1：注入 sw8 trace header（tracer=nil 时降级跳过）
	if p.tracer != nil {
		var sw8 string
		injector := func(key, value string) error {
			if key == sw8HeaderName && value != "" {
				sw8 = value
			}
			return nil
		}
		// CreateExitSpan 可能在 nil receiver / nil tracer 时返 noop span + nil err
		_, _, _ = p.tracer.CreateExitSpan(ctx, "kafka-publish", topic, injector)
		if sw8 != "" {
			msg.Headers = append(msg.Headers, sarama.RecordHeader{
				Key: []byte(sw8HeaderName), Value: []byte(sw8),
			})
		}
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