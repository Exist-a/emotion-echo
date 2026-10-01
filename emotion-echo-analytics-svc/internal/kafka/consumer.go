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
	"sync/atomic"

	"github.com/emotion-echo/shared/pkg/logging"
	"time"

	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"

	"github.com/IBM/sarama"
	"github.com/emotion-echo/shared/pkg/eventrow"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	sharedmessaging "github.com/emotion-echo/shared/pkg/messaging"
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
	dlq     DLQPublisher
	retries *atomic.Int32
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

	// E2E-23 #32：重试次数改为 atomic 容器，consumer 与 handler **共享同一个**。
	// 此前是构造期拷贝进 handler 的普通 int，Nacos 推新值只改得到 consumer
	// 那一侧，消费判定仍读旧值 ⇒ 又一个"配了不生效"（账本 F-160）。
	var retries atomic.Int32
	retries.Store(3) // 默认 3，与 consumer.go:69,71 的历史硬编码一致

	c := &Consumer{
		topic:    topic,
		groupID:  groupID,
		brokers:  brokers,
		repo:     repo,
		client:   client,
		consumer: &chatEventHandler{repo: repo, topic: topic, dlq: NoopDLQPublisher{}, retries: &retries},
		dlq:      NoopDLQPublisher{},
		retries:  &retries,
	}
	return c, nil
}

// WithDLQ 设置 DLQ publisher（builder 模式）
func (c *Consumer) WithDLQ(dlq DLQPublisher) *Consumer {
	if dlq != nil {
		c.dlq = dlq
		c.consumer = &chatEventHandler{repo: c.repo, topic: c.topic, dlq: dlq, retries: c.retries}
	}
	return c
}

// currentMaxRetries 读当前重试上限（consumer 与 handler 共用同一容器）。
func (c *Consumer) currentMaxRetries() int {
	if c == nil || c.retries == nil {
		return 0
	}
	return int(c.retries.Load())
}

// CurrentMaxRetries 导出当前重试上限（供 Nacos ops 回调日志与测试用）。
func (c *Consumer) CurrentMaxRetries() int { return c.currentMaxRetries() }

// UpdateMaxRetries 运行期更新重试上限（Nacos ops 回调调用）。
//
// 非正值被忽略：0 会变成"永不重试"（一旦入 DLQ 就再无重试），
// 与 main.go 的 `> 0` 守卫语义保持一致。
func (c *Consumer) UpdateMaxRetries(n int) {
	if c == nil || c.retries == nil || n <= 0 {
		return
	}
	c.retries.Store(int32(n))
}

// currentMaxRetries handler 侧的读取（与 Consumer 共享容器）。
func (h *chatEventHandler) currentMaxRetries() int {
	if h == nil || h.retries == nil {
		return 0
	}
	return int(h.retries.Load())
}

// WithMaxRetries 设置最大重试次数
func (c *Consumer) WithMaxRetries(n int) *Consumer {
	if n > 0 {
		c.retries.Store(int32(n))
		c.consumer = &chatEventHandler{repo: c.repo, topic: c.topic, dlq: c.dlq, retries: c.retries}
	}
	return c
}

// WithTracer 注入 SkyWalking tracer（builder 模式，与 WithDLQ / WithMaxRetries 对称）
//
// Stage 93 PR-1: 从 chat-svc producer (Stage 92 PR-1) 写入的 Kafka sw8 header 抽回
// 重建父 trace。tracer=nil 时不注入,与 Stage 30-A Round 4 原行为一致（向后兼容）。
func (c *Consumer) WithTracer(tracer grpcinterceptor.Tracer) *Consumer {
	c.consumer = &chatEventHandler{
		repo:    c.repo,
		topic:   c.topic,
		dlq:     c.dlq,
		retries: c.retries,
		Tracer:  tracer,
	}
	return c
}

// Run 启动 consumer；ctx 取消时退出。
//
// 失败语义：topic 不存在 / broker 不可达 — log warn + 继续运行
// （不阻塞 HTTP server）。业务通过 ctx.Cancel 触发优雅退出。
func (c *Consumer) Run(ctx context.Context) error {
	for {
		if err := c.client.Consume(ctx, []string{c.topic}, c.consumer); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			logging.PrintfContext(ctx, "[kafka-consumer] consume error (will retry in 5s): %v", err)
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
	repo    repository.EventRepo
	topic   string
	dlq     DLQPublisher
	retries *atomic.Int32

	// E2E-F-174（2026-10-01）：重试退避注入点。nil = 默认指数退避
	// min(2^attempt 秒, 30s)。测试注入 0 退避以保证确定性。
	backoffFn func(attempt int) time.Duration

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
// E2E-F-174（2026-10-01 重写）：handleOne 失败的消息**在同一条消息上原地重试**
// （deliverWithRetry，预算 = MaxRetries+1 次尝试），预算耗尽 → DLQ + Mark。
// 旧实现的"重试"不回退 offset、计数靠后续同 key 消息失败次数推进 ⇒ 单条毒消息
// 永不被重新处理、可永久阻塞分区，且计数跨重启清零（账本 D3/F-174 实测）。
// 新实现下每条消息的结果（落库 or DLQ）在单次遍历内确定，分区永不因单条消息卡死。
func (h *chatEventHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			// Stage 93 PR-1 + Stage 94 PR-2b: span 创建前先 DecodeChatEvent 拿到 evt.Type
			// （让 4 个 messaging.* tag 用精确值，不用 msg.Topic 兜底）
			// span 提到 case 顶,不用 defer —— 避免 N 条消息 span 累积到 ConsumeClaim 退出
			var span grpcinterceptor.Span
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
					_, s, err := h.Tracer.CreateEntrySpan(sess.Context(), "kafka-consume", extractor)
					if err != nil {
						log.Printf("[kafka-consumer] create entry span failed (continuing without trace): %v", err)
					}
					span = s
					if span != nil {
						span.Tag("messaging.system", "kafka")
						span.Tag("messaging.kafka.topic", msg.Topic)
						span.Tag("messaging.kafka.partition", fmt.Sprintf("%d", msg.Partition))
						span.Tag("event.type", evt.Type)
					}
				} else {
					log.Printf("[kafka-consumer] decode failed (skip span, handleOne will retry): %v", decodeErr)
				}
			}
			attempts, handlerErr := h.deliverWithRetry(msg)
			if handlerErr != nil {
				h.publishToDLQ(sess, msg, handlerErr, attempts)
			}
			// 落库成功 或 重试预算耗尽（已进 DLQ）→ 都 Mark 前进，
			// 分区消费不因单条消息停滞。
			sess.MarkMessage(msg, "")
			// Stage 94 PR-2b §P0-3：case 末尾立刻 EndSpan（不用 defer —— 绑定到
			// ConsumeClaim 函数返回会让 N 条消息 span 累积到 consumer 退出才收尾）
			if span != nil {
				span.EndSpan(handlerErr)
			}
		case <-sess.Context().Done():
			return nil
		}
	}
}

// deliverWithRetry 在**同一条消息**上原地重试 handleOne，直到成功或预算耗尽。
//
// 预算 = currentMaxRetries()+1 次尝试（与旧语义的 DLQ.Attempts=4 对齐）。
// 相邻尝试之间按 backoffFor(attempt) 退避；handleOne 无内部状态，天然可重入。
// 返回 (实际尝试次数, 最终 error)；成功时 error 为 nil。
func (h *chatEventHandler) deliverWithRetry(msg *sarama.ConsumerMessage) (int, error) {
	maxRetries := h.currentMaxRetries()
	var err error
	for attempt := 1; ; attempt++ {
		if err = h.handleOne(msg); err == nil {
			return attempt, nil
		}
		if attempt > maxRetries {
			return attempt, err
		}
		d := h.backoffFor(attempt)
		log.Printf("[kafka-consumer] handle %s failed (will retry attempt=%d/%d offset=%d, backoff=%s): %v",
			string(msg.Key), attempt, maxRetries, msg.Offset, d, err)
		time.Sleep(d)
	}
}

// backoffFor 第 attempt 次失败后的等待时长：注入的 backoffFn 优先；
// 默认指数退避 min(2^attempt 秒, 30s)——2s/4s/8s…，给下游（DB/网络）恢复窗口。
func (h *chatEventHandler) backoffFor(attempt int) time.Duration {
	if h.backoffFn != nil {
		return h.backoffFn(attempt)
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// publishToDLQ 重试预算耗尽后的兜底：投 DLQ（带诊断 headers + sw8 透传）+ 计数。
// 无论 DLQ 是否配置（Noop），调用方都必须 Mark 前进 —— 由 ConsumeClaim 统一处理。
func (h *chatEventHandler) publishToDLQ(sess sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage, lastErr error, attempts int) {
	if h.dlq != nil {
		// P1-2 (Round 1): 透传原 headers（含 sw8），让 OAP 端能继续追 trace。
		dlqHeaders := make(map[string]string, len(msg.Headers))
		for _, hd := range msg.Headers {
			dlqHeaders[string(hd.Key)] = string(hd.Value)
		}
		dlqEntry := DLQEntry{
			Topic:         msg.Topic,
			Key:           msg.Key,
			Value:         msg.Value,
			Attempts:      attempts,
			LastError:     lastErr.Error(),
			OriginalTopic: msg.Topic,
			Headers:       dlqHeaders,
		}
		if dlqErr := h.dlq.Publish(sess.Context(), dlqEntry); dlqErr != nil {
			// Round 2.3 §PR-1: analytics-svc DLQ 投递失败计数（kafka-pipeline-pending-decisions.md §P1-14）。
			IncDLQPublishResult(false)
			log.Printf("[kafka-consumer] DLQ publish failed (dropping msg): %v", dlqErr)
		} else {
			IncDLQPublishResult(true)
		}
	}
	log.Printf("[kafka-consumer] handle %s failed after %d attempts → DLQ: %v",
		string(msg.Key), attempts, lastErr)
}

// extractSw8Header 从 sarama RecordHeader 列表抽 sw8 header value
//
// Stage 93 PR-1: chat-svc producer (Stage 92 PR-1) 写到 Kafka header["sw8"] 的
// 字符串由此函数抽回 → 喂给 Tracer.CreateEntrySpan 的 extractor → go2sky 重建父 trace。
//
// header 名常量与 chat-svc kafka_publisher.sw8HeaderName 一致 ("sw8")。
// 这里不复用 shared 常量(避免 analytics-svc 引入 chat-svc 才有的传递依赖;
// 函数体与 ai-svc internal/consumer/consumer.go:208-218 完全对称）。
//
// Round B: 收敛到 shared/pkg/messaging.ExtractSw8Header（kafka-pipeline
// D8-2 登记的"双份未收敛"项）。本处保留薄包装供本地调用方不破改动。
func extractSw8Header(headers []*sarama.RecordHeader) string {
	return sharedmessaging.ExtractSw8Header(headers)
}

// handleFailure / attemptKey 已删除（E2E-F-174，2026-10-01）：
// 旧"重试"不重投消息、按 msg.Key 统计后续消息失败次数，语义失真（F-174/D3 实测）。
// 替代物 = deliverWithRetry（原地重试）+ publishToDLQ（预算耗尽兜底）。

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
	// E2E-F-146：Kafka 消息没有 ctx，但 record header 里带着 sw8（chat-svc 发布时
	// 写入，kafka_publisher.go:61-63）。解出 trace_id 塞进 ctx，**本函数及其
	// 下游全部日志**就能和发布侧链路对上 —— 异步链路恰恰最需要 trace
	// （"这条消息是谁发的、后来怎么了"），没有它就彻底断开。
	ctx := context.Background()
	if tid := traceIDFromMessage(msg); tid != "" {
		ctx = logging.WithTraceID(ctx, tid)
	}

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
			logging.PrintfContext(ctx, "[kafka-consumer] unknown event type %q, skip", ev.Type)
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

// traceIDFromMessage 从 Kafka record header 的 sw8 解出 trace_id。
//
// 解不出（无 sw8 / 格式不对）时返回 ""，调用方据此不写 trace_id 字段 ——
// 绝不返回半截 ID，半截 ID 在 Loki 里查不到任何东西，比没有更误导。
func traceIDFromMessage(msg *sarama.ConsumerMessage) string {
	if msg == nil {
		return ""
	}
	for _, h := range msg.Headers {
		if string(h.Key) == grpcinterceptor.SW8HeaderName {
			return grpcinterceptor.TraceIDFromSW8(string(h.Value))
		}
	}
	return ""
}
