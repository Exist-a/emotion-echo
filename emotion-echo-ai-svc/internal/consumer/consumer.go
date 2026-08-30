// Package consumer 提供 ai-svc 的 Kafka 消费能力
//
// 职责：
//   - 从 chat-events topic 消费 message.created 事件
//   - 调用 analyzer 跑情绪分析
//   - 写 emotion_analysis 表
//
// 设计：
//   - Consumer 接口 → KafkaConsumer (生产) / InMemoryConsumer (测试)
//   - Handler 函数签名简单：ctx + event → 处理结果
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/logging"

	"github.com/IBM/sarama"
	"github.com/SkyAPM/go2sky"
)

// ConsumerGroupHandler 是 sarama.ConsumerGroupHandler 的实现
//
// 收到消息后：
//  1. 解析为 Event
//  2. 调用 Handler（可选创建 SkyWalking span）
//  3. 标记消费成功（返回 nil）
//
// Stage 30-C A2: Handler 返 error 时不再无限重投
//   - attempt < MaxRetries：递增计数，不 MarkMessage（sarama 自动重投）
//   - attempt >= MaxRetries：调 DLQ.Publish + MarkMessage + 重置计数
//   - DLQ 为 nil 时退化为原行为（不 MarkMessage，保留向后兼容）
type ConsumerGroupHandler struct {
	// Ready 当 setup 完成后会关闭这个 channel
	Ready chan bool
	// Handler 业务处理函数：消费事件并返回 error
	Handler MessageHandler
	// TopicFilter 仅处理匹配的事件类型（如 "message.created"）
	TopicFilter string
	// Tracer 可选：用于创建 SkyWalking span（Stage 25-F）
	// 为 nil 时不创建 span，保证向后兼容
	Tracer *go2sky.Tracer
	// DLQ Stage 30-C A2：可选 DLQ publisher。nil 时不投 DLQ（退化）。
	DLQ DLQPublisher
	// MaxRetries Stage 30-C A2：失败最大重试次数，0 时取默认值 3。
	MaxRetries int
	// attempts Stage 30-C A2：msg.Key → 已重试次数（消费周期内有效）
	attempts map[string]int
}

// MessageHandler 是单条消息的业务处理函数
//
// 返回 nil → 提交 offset
// 返回 error → 不提交，下一轮重试（sarama 默认行为）
type MessageHandler func(ctx context.Context, evt *events.Event) error

// Setup sarama callback：进入新会话时被调用
func (h *ConsumerGroupHandler) Setup(sess sarama.ConsumerGroupSession) error {
	close(h.Ready)
	return nil
}

// Cleanup sarama callback：会话结束时被调用
func (h *ConsumerGroupHandler) Cleanup(sess sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 实际消费消息
//
// Stage 25-F：当 h.Tracer 非 nil 时，为每条消息创建 SkyWalking local span，
// 标签包含 messaging.system / topic / partition / event.type，便于 SkyWalking UI 聚合分析。
//
// Stage 30-C A2 (RED step — 仅声明接口字段，handleFailure 由 commit 6 GREEN 落地)
// 当前行为保持与 Stage 30-B 一致：Handler 返 error → 不 MarkMessage。
func (h *ConsumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			// 解析事件
			var evt events.Event
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				logging.Errorf(err, "[consumer] unmarshal err (skip)")
				sess.MarkMessage(msg, "")
				continue
			}
			// 类型过滤
			if h.TopicFilter != "" && evt.Type != h.TopicFilter {
				sess.MarkMessage(msg, "")
				continue
			}
			// Stage 25-F: SkyWalking span（可选）
			if h.Tracer != nil {
				span, _, _ := h.Tracer.CreateLocalSpan(sess.Context(),
					go2sky.WithOperationName("kafka-consume"))
				if span != nil {
					defer span.End()
					span.Tag("messaging.system", "kafka")
					span.Tag("messaging.kafka.topic", msg.Topic)
					span.Tag("messaging.kafka.partition", fmt.Sprintf("%d", msg.Partition))
					span.Tag("event.type", evt.Type)
				}
			}
			// 调业务
			if err := h.Handler(sess.Context(), &evt); err != nil {
				logging.Errorf(err, "[consumer] handler err")
				// 不 MarkMessage，让 sarama 在重试后再次投递
				continue
			}
			sess.MarkMessage(msg, "")
		case <-sess.Context().Done():
			return nil
		}
	}
}

// =====================================================
// KafkaConsumer（生产实现）
// =====================================================

// KafkaConsumer 封装 sarama ConsumerGroup
type KafkaConsumer struct {
	group   sarama.ConsumerGroup
	topics  []string
	mu      sync.Mutex
	started bool
}

// NewKafkaConsumer 创建 Kafka consumer
func NewKafkaConsumer(brokers []string, groupID string) (*KafkaConsumer, error) {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Return.Errors = true
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest // 从最早开始
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, err
	}
	return &KafkaConsumer{group: group}, nil
}

// Consume 阻塞消费 topic，直到 ctx 取消
//
// 真正的 sarama ConsumerGroup.Consume 内部循环处理 rebalance
//
// 参数 tracer 可选：传入后每条消息会创建 SkyWalking span（Stage 25-F）。
func (c *KafkaConsumer) Consume(ctx context.Context, topics []string, handler MessageHandler, topicFilter string, tracer *go2sky.Tracer) error {
	c.topics = topics
	h := &ConsumerGroupHandler{
		Ready:       make(chan bool),
		Handler:     handler,
		TopicFilter: topicFilter,
		Tracer:      tracer,
	}

	// 阻塞循环：每次 Consume 返回时（rebalance 或错误）重试
	for {
		if err := c.group.Consume(ctx, topics, h); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			logging.Errorf(err, "[consumer] consume err")
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// Close 关闭 consumer
func (c *KafkaConsumer) Close() error {
	return c.group.Close()
}