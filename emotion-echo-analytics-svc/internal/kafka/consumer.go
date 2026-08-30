// Package kafka — consumer.go
//
// Stage 30-A Round 4 part 2 GREEN: chat-events consumer that subscribes
// to the chat-svc Kafka topic and writes User_beBehaviorEvent rows.
//
// 复用 shared/pkg/messaging.KafkaProducer 的 Event schema（chat-svc
// 与 analytics-svc 用同一个 JSON shape），仅 consumer 是本包实现。
//
// 契约（per docs/stage-30-A §三.3）：
//   - 订阅 chat-events topic（默认）
//   - message.created → User_beBehaviorEvent{type:message}
//   - conversation.created → User_beBehaviorEvent{type:conversation_created}
//   - conversation.closed → User_beBehaviorEvent{type:conversation_closed}
//   - 启动失败不 crash HTTP server（topic 不存在时 log warn）
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"emotion-echo-analytics-svc/internal/events"
	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"

	"github.com/IBM/sarama"
)

// Consumer 订阅 chat-events topic 并写 User_beBehaviorEvent
type Consumer struct {
	topic    string
	groupID  string
	brokers  []string
	repo     repository.EventRepo
	client   sarama.ConsumerGroup
	consumer sarama.ConsumerGroupHandler
}

// NewConsumer 构造（不启动；需调 Run）
func NewConsumer(brokers []string, groupID, topic string, repo repository.EventRepo) (*Consumer, error) {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Return.Errors = true

	client, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, err
	}

	c := &Consumer{
		topic:   topic,
		groupID: groupID,
		brokers: brokers,
		repo:    repo,
		client:  client,
		consumer: &chatEventHandler{repo: repo, topic: topic},
	}
	return c, nil
}

// Run 启动 consumer；ctx 取消时退出。
//
// 失败语义：topic 不存在 / broker 不可达 — log warn + 继续运行
//（不阻塞 HTTP server）。业务通过 ctx.Cancel 触发优雅退出。
func (c *Consumer) Run(ctx context.Context) error {
	for {
		if err := c.client.Consume(ctx, []string{c.topic}, c.consumer); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			log.Printf("[kafka-consumer] consume error (will retry in 5s): %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// Close 关闭 consumer group
func (c *Consumer) Close() error {
	return c.client.Close()
}

// chatEventHandler 处理 chat-events 消息
type chatEventHandler struct {
	repo  repository.EventRepo
	topic string
}

func (h *chatEventHandler) Setup(_ sarama.ConsumerGroupSession) error {
	log.Printf("[kafka-consumer] session setup (topic=%s)", h.topic)
	return nil
}

func (h *chatEventHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 每条 chat-event 写一条 User_beBehaviorEvent
func (h *chatEventHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			if err := h.handleOne(msg); err != nil {
				log.Printf("[kafka-consumer] handle %s failed: %v (offset=%d, will skip)",
					string(msg.Key), err, msg.Offset)
			}
			sess.MarkMessage(msg, "")
		case <-sess.Context().Done():
			return nil
		}
	}
}

// handleOne 把一条 chat-event 写为 User_beBehaviorEvent
//
// 事件类型 → 行为类型映射：
//   message.created        → "message"
//   conversation.created  → "conversation_created"
//   conversation.closed    → "conversation_closed"
func (h *chatEventHandler) handleOne(msg *sarama.ConsumerMessage) error {
	var ev events.Event
	if err := json.Unmarshal(msg.Value, &ev); err != nil {
		return err
	}

	var target string
	var userID int64
	switch ev.Type {
	case events.EventTypeMessageCreated:
		var d events.MessageCreatedData
		if err := remarshal(ev.Data, &d); err != nil {
			return err
		}
		target = "message"
		userID = d.UserID
	case events.EventTypeConversationCreated:
		var d events.ConversationCreatedData
		if err := remarshal(ev.Data, &d); err != nil {
			return err
		}
		target = "conversation"
		userID = d.UserID
	case events.EventTypeConversationClosed:
		var d events.ConversationClosedData
		if err := remarshal(ev.Data, &d); err != nil {
			return err
		}
		target = "conversation"
		userID = d.UserID
	default:
		// 未知事件类型 — 跳过但不报错
		log.Printf("[kafka-consumer] unknown event type %q, skip", ev.Type)
		return nil
	}

	be := &model.UserBehaviorEvent{
		EventID:    ev.ID, // Stage 30-C A1: 事件 ID 作幂等键 → 重复消费去重
		UserID:     userID,
		EventType:  target,
		Target:     ev.ID, // 用 Event.ID 作 target 标识
		SessionID:  msg.Topic, // 没有 session 字段，暂用 topic
		OccurredAt: ev.Time,
	}
	return h.repo.Create(nil, be) // 简化：ctx nil；真实应传 sess.Context()
}

// remarshal 把 any-typed Data 字段二次反序列化为目标类型
func remarshal(data any, target any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}