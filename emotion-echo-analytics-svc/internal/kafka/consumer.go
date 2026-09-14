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
	"fmt"
	"log"
	"sync"
	"time"

	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"

	"github.com/IBM/sarama"
	"github.com/emotion-echo/shared/pkg/eventrow"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

// Consumer 订阅 chat-events topic 并写 User_beBehaviorEvent
type Consumer struct {
	topic    string
	groupID  string
	brokers  []string
	repo     repository.EventRepo
	client   sarama.ConsumerGroup
	consumer sarama.ConsumerGroupHandler

	// Stage 30-C A2: DLQ 注入与重试配置
	dlq        DLQPublisher
	maxRetries int
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
		topic:      topic,
		groupID:    groupID,
		brokers:    brokers,
		repo:       repo,
		client:     client,
		consumer:   &chatEventHandler{repo: repo, topic: topic, dlq: NoopDLQPublisher{}, maxRetries: 3},
		dlq:        NoopDLQPublisher{},
		maxRetries: 3,
	}
	return c, nil
}

// WithDLQ 设置 DLQ publisher（builder 模式）
func (c *Consumer) WithDLQ(dlq DLQPublisher) *Consumer {
	if dlq != nil {
		c.dlq = dlq
		c.consumer = &chatEventHandler{repo: c.repo, topic: c.topic, dlq: dlq, maxRetries: c.maxRetries}
	}
	return c
}

// WithMaxRetries 设置最大重试次数
func (c *Consumer) WithMaxRetries(n int) *Consumer {
	if n > 0 {
		c.maxRetries = n
		c.consumer = &chatEventHandler{repo: c.repo, topic: c.topic, dlq: c.dlq, maxRetries: n}
	}
	return c
}

// WithTracer 注入 SkyWalking tracer（builder 模式，与 WithDLQ / WithMaxRetries 对称）
//
// Stage 93 PR-1: 从 chat-svc producer (Stage 92 PR-1) 写入的 Kafka sw8 header 抽回
// 重建父 trace。tracer=nil 时不注入,与 Stage 30-A Round 4 原行为一致（向后兼容）。
func (c *Consumer) WithTracer(tracer grpcinterceptor.Tracer) *Consumer {
	c.consumer = &chatEventHandler{
		repo:       c.repo,
		topic:      c.topic,
		dlq:        c.dlq,
		maxRetries: c.maxRetries,
		Tracer:     tracer,
	}
	return c
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
	repo       repository.EventRepo
	topic      string
	dlq        DLQPublisher
	maxRetries int
	// attempts msg.Key → 重试次数（消费周期内）
	//
	// Round 5b §B: attemptsMu 守卫 map 读写。sarama 当前 ConsumeClaim 是单
	// goroutine(内部保证),但未来重构或 sarama 跨 goroutine 派发 partition 时
	// map 会触发 race detector。加 sync.Mutex 防御性保护。
	attempts   map[string]int
	attemptsMu sync.Mutex

	// Stage 93 PR-1: 可选 SkyWalking tracer（grpcinterceptor.Tracer 接口,
	// PR-OBS-17 + Stage 92 PR-1 扩展)。非 nil 时每条消息走 CreateEntrySpan
	// 从 msg.Headers[sw8] 重建父 trace（chat-svc producer → analytics-svc consumer
	// 跨进程 trace）。nil 时跳过 span 创建（向后兼容 Stage 30-A Round 4）。
	Tracer grpcinterceptor.Tracer
}

func (h *chatEventHandler) Setup(_ sarama.ConsumerGroupSession) error {
	log.Printf("[kafka-consumer] session setup (topic=%s)", h.topic)
	return nil
}

func (h *chatEventHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 每条 chat-event 写一条 User_behaviorEvent
//
// Stage 30-C A2: handleOne 返 error → 走 attempt 计数 → 超 MaxRetries 投 DLQ。
//   - attempt <= MaxRetries：返回 error 让 sarama 不 Mark（自动重投）
//   - attempt > MaxRetries：调 DLQ.Publish + Mark + 清 attempts
//   - DLQ=NoopDLQPublisher 时等价于"无 DLQ 兜底"，仍走 attempt 计数（避免毒消息卡死）
//
// Stage 93 PR-1: 当 h.Tracer 非 nil 时,每条消息调 CreateEntrySpan 从 msg.Headers[sw8]
// 重建父 trace（chat-svc producer → analytics-svc consumer 跨进程 trace）。
//   - msg 含 sw8 header → extractor 抽到 → go2sky 重建父 SpanContext
//   - msg 无 sw8 header → extractor 返 "" → go2sky Valid=false → 新 trace 起点
//   - Tracer=nil → 完全跳过 span 创建（Stage 30-A Round 4 原行为,向后兼容）
//
// 与 ai-svc Stage 92 PR-2 同模式 (consumer.go:115-134)：先 DecodeChatEvent 抽出 evt,
// 再创建 span 并打 4 个 messaging.* tag (含 event.type),最后调 handleOne 写库。
// 解析失败时仍走 handleOne 自身错误路径(返回 error 走 attempt 计数)。
func (h *chatEventHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	h.attemptsMu.Lock()
	if h.attempts == nil {
		h.attempts = make(map[string]int)
	}
	h.attemptsMu.Unlock()
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			// Stage 93 PR-1: SkyWalking span (可选) —— 与 ai-svc 同模式。
			// 解析 evt 提前到 span 创建之前,4 个 messaging.* tag 用 evt.Type 精确值
			// （而不是 msg topic 兜底,后者不区分 conversation.created/message.created）。
			// 解析失败时跳过 span 创建(走 handleOne 自身 error 路径,与 ai-svc 一致)。
			if h.Tracer != nil {
				evt, decodeErr := DecodeChatEvent(msg.Value, saramaHeaders(msg))
				if decodeErr == nil {
					sw8Header := extractSw8Header(msg.Headers)
					extractor := func(key string) (string, error) {
						if key == "sw8" {
							return sw8Header, nil
						}
						return "", nil
					}
					_, span, err := h.Tracer.CreateEntrySpan(sess.Context(), "kafka-consume", extractor)
					if err != nil {
						log.Printf("[kafka-consumer] create entry span failed (continuing without trace): %v", err)
					}
					if span != nil {
						defer span.EndSpan(nil)
						span.Tag("messaging.system", "kafka")
						span.Tag("messaging.kafka.topic", msg.Topic)
						span.Tag("messaging.kafka.partition", fmt.Sprintf("%d", msg.Partition))
						span.Tag("event.type", evt.Type)
					}
				} else {
					log.Printf("[kafka-consumer] decode failed (skip span, handleOne will retry): %v", decodeErr)
				}
			}
			if err := h.handleOne(msg); err != nil {
				h.handleFailure(sess, msg, err)
				continue
			}
			// 业务成功：清 attempts
			if key := string(msg.Key); key != "" {
				h.attemptsMu.Lock()
				delete(h.attempts, key)
				h.attemptsMu.Unlock()
			}
			sess.MarkMessage(msg, "")
		case <-sess.Context().Done():
			return nil
		}
	}
}

// extractSw8Header 从 sarama RecordHeader 列表抽 sw8 header value
//
// Stage 93 PR-1: chat-svc producer (Stage 92 PR-1) 写到 Kafka header["sw8"] 的
// 字符串由此函数抽回 → 喂给 Tracer.CreateEntrySpan 的 extractor → go2sky 重建父 trace。
//
// header 名常量与 chat-svc kafka_publisher.sw8HeaderName 一致 ("sw8")。
// 这里不复用 shared 常量(避免 analytics-svc 引入 chat-svc 才有的传递依赖;
// 函数体与 ai-svc internal/consumer/consumer.go:208-218 完全对称)。
func extractSw8Header(headers []*sarama.RecordHeader) string {
	for _, hdr := range headers {
		if hdr == nil {
			continue
		}
		if string(hdr.Key) == "sw8" {
			return string(hdr.Value)
		}
	}
	return ""
}

// handleFailure 处理 handleOne 失败（Stage 30-C A2）
func (h *chatEventHandler) handleFailure(sess sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage, handlerErr error) {
	key := attemptKey(msg)
	// Round 5b §B: 读写 attempts 加 sync.Mutex 守卫(防御性,跨 goroutine 安全)
	h.attemptsMu.Lock()
	if h.attempts == nil {
		h.attempts = make(map[string]int)
	}
	h.attempts[key]++
	attempt := h.attempts[key]
	h.attemptsMu.Unlock()

	if attempt <= h.maxRetries {
		log.Printf("[kafka-consumer] handle %s failed (will retry attempt=%d/%d offset=%d): %v",
			string(msg.Key), attempt, h.maxRetries, msg.Offset, handlerErr)
		return
	}

	// 已达最大重试 → DLQ + Mark
	if h.dlq != nil {
		dlqEntry := DLQEntry{
			Topic:         msg.Topic,
			Key:           msg.Key,
			Value:         msg.Value,
			Attempts:      attempt,
			LastError:     handlerErr.Error(),
			OriginalTopic: msg.Topic,
		}
		if dlqErr := h.dlq.Publish(sess.Context(), dlqEntry); dlqErr != nil {
			log.Printf("[kafka-consumer] DLQ publish failed (dropping msg): %v", dlqErr)
		}
	}
	log.Printf("[kafka-consumer] handle %s failed after %d retries → DLQ: %v",
		string(msg.Key), attempt, handlerErr)
	h.attemptsMu.Lock()
	delete(h.attempts, key)
	h.attemptsMu.Unlock()
	sess.MarkMessage(msg, "")
}

// attemptKey 取 msg.Key，无 key 时用 partition:offset 兜底
func attemptKey(msg *sarama.ConsumerMessage) string {
	if len(msg.Key) > 0 {
		return string(msg.Key)
	}
	return fmt.Sprintf("%d:%d", msg.Partition, msg.Offset)
}

// handleOne 把一条 chat-event 写为 User_behaviorEvent
//
// ADR-19 PR-A1.4 (Sprint A 全收口): event_type 落库值也统一 chat-svc 原值
// (带点 message.created / conversation.created / conversation.closed),
// 不再 normalizeEventType。target/session_id (PR-A1.3 v2) + event_type
// (PR-A1.4) 都走 chat-svc 风格 → analytics-svc consumer 与 chat-svc
// DevEventPublisher 真正"消灭两份映射"。
//
// 历史 normalize 后值(message/conversation_created/conversation_closed)的数据
// 由 migrations/002_create_user_behavior_events.sql 末尾的 ADR-19 数据迁移
// SQL 段负责一次性 UPDATE。
func (h *chatEventHandler) handleOne(msg *sarama.ConsumerMessage) error {
	// Stage 73：Protobuf 优先 + 旧 JSON fallback（双写窗口）
	ev, err := DecodeChatEvent(msg.Value, saramaHeaders(msg))
	if err != nil {
		return err
	}

	shape, err := extractDataShape(ev.Data)
	if err != nil {
		return err
	}

	// PR-A1.4: event_type 直接用 ev.Type 原值（带点），不再 normalize
	row, err := eventrow.MapEventToUserBehaviorRow(
		ev.ID, ev.Type, shape, ev.Time,
	)
	if err != nil {
		if errors.Is(err, eventrow.ErrUnknownEventType) {
			log.Printf("[kafka-consumer] unknown event type %q, skip", ev.Type)
			return nil
		}
		return err
	}

	be := &model.UserBehaviorEvent{
		EventID:    row.EventID,
		UserID:     row.UserID,
		EventType:  row.EventType,
		Target:     row.Target,
		SessionID:  row.SessionID,
		OccurredAt: row.OccurredAt,
	}
	return h.repo.Create(nil, be)
}

// extractDataShape 从 events.Data (any) 抽取 eventrow.DataShape 字段
//
// 与 chat-svc dev_publisher.go extractDataShape 同构(都从 JSON tag 抽取
// messageId/conversationId/userId)。shared/eventrow 不反向 import events 包,
// 所以两端各自实现这个适配层。
func extractDataShape(data any) (eventrow.DataShape, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return eventrow.DataShape{}, fmt.Errorf("kafka-consumer: marshal data: %w", err)
	}
	var dyn map[string]any
	if err := json.Unmarshal(b, &dyn); err != nil {
		return eventrow.DataShape{}, fmt.Errorf("kafka-consumer: unmarshal data to dyn: %w", err)
	}
	shape := eventrow.DataShape{}
	if v, ok := dyn["messageId"]; ok {
		if f, ok := v.(float64); ok {
			shape.MessageID = int64(f)
		}
	}
	if v, ok := dyn["conversationId"]; ok {
		if f, ok := v.(float64); ok {
			shape.ConversationID = int64(f)
		}
	}
	if v, ok := dyn["userId"]; ok {
		if f, ok := v.(float64); ok {
			shape.UserID = int64(f)
		}
	}
	return shape, nil
}

// normalizeEventType 已废弃 (ADR-19 PR-A1.4 Sprint A 全收口)
//   - Sprint A 收口前: 把 ev.Type 转 normalize 后值("message.created" → "message")
//     避免 Stage 30-C 之前的"conversation"合并bug复发
//   - Sprint A 收口后: 直接用 ev.Type 原值(带点),与 chat-svc DevEventPublisher
//     完全一致 → 真正消灭两份映射
//
// 函数已删除(handleOne 不再调用)。历史数据由 migrations/002 末尾的
// ADR-19 数据迁移 SQL 段负责一次性 UPDATE normalize 后值 → 带点原值。

// remarshal 把 any-typed Data 字段二次反序列化为目标类型
func remarshal(data any, target any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}