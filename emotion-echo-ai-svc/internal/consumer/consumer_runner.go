// Package consumer — Round 4.7 §F：consumer.go 第二轮拆分
//
// 本文件包含 KafkaConsumer（生产实现）—— 从 consumer.go 迁出。
// consumer.go 现在只剩 ConsumerGroupHandler（Handler 接口实现）+ Setup/Cleanup/
// ConsumeClaim，让主文件 < 200 行。
package consumer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

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
// Stage 30-C A2: dlq 可选 — 注入后启用 DLQ 路径。nil 时退化为 Stage 30-B 原行为。
//
// PR-OBS-17: tracer 参数类型从 *go2sky.Tracer 改为 grpcinterceptor.Tracer 接口,
// 调用方需用 grpcinterceptor.NewGo2SkyTracer(t) 包一层。
//
// ADR-19 PR-A2.1: 外层 5s 重试对齐 analytics-svc Consumer.Run
//
// 历史 bug：session 级故障（consumer group 被关闭、broker 长期不可达超过 sarama
// 内部重试）时 Consume 返 err,本函数直接 return → 调用方 goroutine 死掉,只能
// 重启进程。修复:对齐 analytics-svc internal/kafka/consumer.go:94 Run 的模式
// —— 出错 log + sleep 5s + continue(除非 ctx 取消或 sarama.ErrClosedConsumerGroup)
//
// 与 analytics-svc 的差异:本服务单 topic 消费 + sarama.ConsumerGroupHandler
// 路径,内部行为对齐即可;Sleep 时长与 analytics-svc 保持一致,便于未来提取到
// shared pkg 重试 helper。
func (c *KafkaConsumer) Consume(ctx context.Context, topics []string, handler MessageHandler, topicFilter string, tracer grpcinterceptor.Tracer, dlq DLQPublisher, maxRetries int) error {
	c.topics = topics
	h := &ConsumerGroupHandler{
		Ready:       make(chan bool),
		Handler:     handler,
		TopicFilter: topicFilter,
		Tracer:      tracer,
		DLQ:         dlq,
		MaxRetries:  maxRetries,
	}

	// 阻塞循环：每次 Consume 返回时（rebalance 或错误）重试
	for {
		if err := c.group.Consume(ctx, topics, h); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			// ADR-19 PR-A2.1: session 级故障不直接 return,5s 后重试
			slog.ErrorContext(ctx, "consumer consume failed (will retry in 5s)",
				"err", err, "topics", topics)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
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
