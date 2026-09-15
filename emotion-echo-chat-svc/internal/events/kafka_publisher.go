// Package events 的 Kafka 生产实现
package events

import (
	"context"
	"log"
	"strconv"
	"time"

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
//
// P1-16 (Round 1): Net.DialTimeout 默认 30s → broker 抖动期间 Gin handler
// 卡 30s。显式压短到 5s + 整体 Producer.Timeout=10s 兜底。
func NewKafkaEventPublisher(brokers []string) (*KafkaEventPublisher, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll          // 强 durability
	cfg.Producer.Retry.Max = 5                              // 5 次重试
	cfg.Producer.Return.Successes = true                   // 同步等待成功
	cfg.Producer.Return.Errors = true                       // 错误回传
	cfg.Producer.Partitioner = sarama.NewHashPartitioner   // 同 key 落同 partition
	cfg.Producer.Timeout = 10 * time.Second                 // 整 Producer 兜底
	cfg.Net.DialTimeout = 5 * time.Second                   // P1-16 单次 dial
	cfg.Net.ReadTimeout = 5 * time.Second
	cfg.Net.WriteTimeout = 5 * time.Second
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
//
// Stage 94 PR-1 §P0-6：CreateExitSpan 返回的 span 必须 EndSpan，否则 go2sky
// reporter 里累积未收尾的 span 实例，OAP 上 producer span duration 永远 = 0
// 或 = 该进程累计 publish 间隔。原实现 `_, _, _ = ...` 三返回值全 discard。
// 现保留 span 返回值，在 SendMessage 返回后立即调 span.EndSpan(sendErr)，
// 让 OAP 上 producer span duration 反映"broker ack 时刻 - 创建 span 时刻"，
// err 透传让 OAP 标记 producer span 失败。
func (p *KafkaEventPublisher) Publish(ctx context.Context, topic string, e *Event) error {
	body, err := MarshalChatEvent(e)
	if err != nil {
		return err
	}
	// P2-11 (Round 1): 分区键改 conversation_id —— 同会话事件落同 partition，
	// 保留分区局部性 + 顺序保证。原用 e.ID (event_id) 跨会话散列到不同 partition，
	// 导致 ai-svc 处理 conversation 上下文时跨 partition join。
	partitionKey := e.ID
	if mcData, ok := e.Data.(MessageCreatedData); ok && mcData.ConversationID > 0 {
		partitionKey = strconv.FormatInt(mcData.ConversationID, 10)
	} else if ccData, ok := e.Data.(ConversationCreatedData); ok && ccData.ConversationID > 0 {
		partitionKey = strconv.FormatInt(ccData.ConversationID, 10)
	}
	msg := &sarama.ProducerMessage{
		Topic:   topic,
		Key:     sarama.StringEncoder(partitionKey),
		Value:   sarama.ByteEncoder(body),
		Headers: []sarama.RecordHeader{
			{Key: []byte("content-type"), Value: []byte(ContentTypeHeaderProto)},
		},
	}
	// P2-14 (Round 1): ctx 取消时跳过 SendMessage — sarama SyncProducer 无原生 ctx 支持，
	// 阻塞在 broker ack 上时 ctx 取消无法中断，但至少避免已取消 ctx 发新请求。
	//
	// Round 2.4: sarama SyncProducer.SendMessage 仍是阻塞同步调用，仅靠 ctx.Err() 预检
	// 不能中断"已发起但未 ack"的 SendMessage。改用 goroutine + select 包裹：
	//   - 启 goroutine 调 SendMessage，把 (partition, offset, err) 送 resultCh
	//   - select 等 resultCh 或 ctx.Done()
	//   - ctx.Done() 先到 → 立即返 ctx.Err()（注意：goroutine 仍会跑完 SendMessage，
	//     sarama 内部会泄漏一次 SendMessage，但 Producer.Timeout=10s 兜底不致死锁）
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Stage 92 PR-1 + Stage 94 PR-1：注入 sw8 trace header + 保留 span 句柄
	// tracer=nil 时降级跳过（保持 dev / in-memory 路径不变）
	var span grpcinterceptor.Span
	if p.tracer != nil {
		var sw8 string
		injector := func(key, value string) error {
			if key == sw8HeaderName && value != "" {
				sw8 = value
			}
			return nil
		}
		// Stage 94 PR-1 §P0-6：保留 span 返回值，SendMessage 之后 EndSpan(sendErr)
		// go2sky 可能返 nil span (noop receiver / adapter 退化)；nil 守卫保护
		_, span, err = p.tracer.CreateExitSpan(ctx, "kafka-publish", topic, injector)
		if err != nil {
			// CreateExitSpan 失败不阻塞 publish —— 与 Stage 92 注释承诺一致
			log.Printf("[kafka] create exit span failed (publish without trace): %v", err)
		}
		if sw8 != "" {
			msg.Headers = append(msg.Headers, sarama.RecordHeader{
				Key: []byte(sw8HeaderName), Value: []byte(sw8),
			})
		}
	}

	// Round 2.4 §P2-14 GREEN：goroutine + select 包裹 SendMessage，
	// 让 ctx.Done() 能在 SendMessage 阻塞期间立即中断 Publish。
	type sendResult struct {
		partition int32
		offset    int64
		err       error
	}
	resultCh := make(chan sendResult, 1) // buffered = goroutine 不阻塞
	go func() {
		partition, offset, sendErr := p.producer.SendMessage(msg)
		resultCh <- sendResult{partition: partition, offset: offset, err: sendErr}
	}()

	var sendErr error
	select {
	case <-ctx.Done():
		// ctx 取消：不要等 SendMessage，立刻返 ctx.Err()。
		// goroutine 仍会跑完 SendMessage —— sarama 内部协程泄漏一次，
		// 由 Producer.Timeout=10s 兜底。下次 Publish 调用照常工作。
		// span 仍需 EndSpan 让 OAP 标记"客户端 ctx 取消"——送 ctx.Err() 让 span 显示错误。
		if span != nil {
			span.EndSpan(ctx.Err())
		}
		return ctx.Err()
	case r := <-resultCh:
		sendErr = r.err
	}

	// Stage 94 PR-1 §P0-6：SendMessage 同步阻塞（WaitForAll）后立刻 EndSpan。
	// span 仍可能为 nil（tracer=nil / adapter noop），守卫即可。
	if span != nil {
		span.EndSpan(sendErr)
	}
	if sendErr != nil {
		log.Printf("[kafka] publish failed: topic=%s err=%v", topic, sendErr)
		return sendErr
	}
	log.Printf("[kafka] published: topic=%s id=%s type=%s", topic, e.ID, e.Type)
	return nil
}

// Close 关闭 producer
func (p *KafkaEventPublisher) Close() error {
	return p.producer.Close()
}