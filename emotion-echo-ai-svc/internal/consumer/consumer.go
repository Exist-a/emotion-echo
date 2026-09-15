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
//   - Tracer 字段为 grpcinterceptor.Tracer 接口(PR-OBS-17):
//     生产传 grpcinterceptor.NewGo2SkyTracer(t),测试传 mock。
package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"emotion-echo-ai-svc/internal/events"

	"github.com/IBM/sarama"
	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
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
	//
	// PR-OBS-17: 类型从 *go2sky.Tracer 改为 grpcinterceptor.Tracer 接口,
	// 生产赋值需用 grpcinterceptor.NewGo2SkyTracer(t) 包一层。
	Tracer grpcinterceptor.Tracer
	// DLQ Stage 30-C A2：可选 DLQ publisher。nil 时不投 DLQ（退化）。
	DLQ DLQPublisher
	// MaxRetries Stage 30-C A2：失败最大重试次数，0 时取默认值 3。
	MaxRetries int
	// attempts Stage 30-C A2：msg.Key → 已重试次数（消费周期内有效）
	//
	// Round 5b §B：attemptsMu 守卫 map 读写。sarama 当前版本 ConsumeClaim
	// 是单 goroutine(SARAMA 内部保证),但未来重构(worker pool / 异步 retry)
	// 或 sarama 跨 goroutine 派发 partition 时,map 会触发 race detector。
	// 加 sync.Mutex 防御性保护 —— 比 sync.Map 简单且对小 map(<1000 key)性能更好。
	attempts   map[string]int
	attemptsMu sync.Mutex
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
// Stage 30-C A2: Handler 返 error → handleFailure：重试计数 + DLQ。
//
// Stage 94 PR-2a §P0-3：方案 A — span 生命周期提到 case 顶部、case 末尾显式
// span.EndSpan(handlerErr)。原 `defer span.EndSpan(nil)` 在 for-loop case 内会
// 延后到 ConsumeClaim 退出才批量收尾，OAP 上每条消息 duration = 整 consumer
// goroutine 寿命（或永不 EndSpan 直到进程退出）。本次修复 + err 透传。
func (h *ConsumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	h.attemptsMu.Lock()
	if h.attempts == nil {
		h.attempts = make(map[string]int)
	}
	h.attemptsMu.Unlock()
	maxRetries := h.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			// 解析事件（Stage 73：Protobuf 优先 + 旧 JSON fallback，双写窗口）
			evt, err := DecodeChatEvent(msg.Value, saramaHeaders(msg))
			if err != nil {
				slog.ErrorContext(sess.Context(), "consumer decode failed (skipping)", "err", err)
				sess.MarkMessage(msg, "")
				continue
			}
			// 类型过滤
			if h.TopicFilter != "" && evt.Type != h.TopicFilter {
				sess.MarkMessage(msg, "")
				continue
			}
			// Stage 92 PR-2: 用 CreateEntrySpan 从 msg.Headers[sw8] 重建父 trace
			// (chat-svc producer → ai-svc consumer 跨进程 trace)。降级语义:
			//   - msg 无 sw8 header → extractor 返 "" → go2sky Valid=false → 新 trace 起点
			//   - Tracer=nil → 完全跳过 span 创建 (Stage 25-F 原行为)
			// Stage 94 PR-2a §P0-3：span 提到 case 顶、不用 defer，case 末尾立刻 EndSpan
			var span grpcinterceptor.Span
			if h.Tracer != nil {
				sw8Header := extractSw8Header(msg.Headers)
				extractor := func(key string) (string, error) {
					if key == "sw8" {
						return sw8Header, nil
					}
					return "", nil
				}
				_, s, err := h.Tracer.CreateEntrySpan(sess.Context(), "kafka-consume", extractor)
				if err != nil {
					slog.WarnContext(sess.Context(), "create entry span failed (continuing without trace)", "err", err)
				}
				span = s
				if span != nil {
					span.Tag("messaging.system", "kafka")
					span.Tag("messaging.kafka.topic", msg.Topic)
					span.Tag("messaging.kafka.partition", fmt.Sprintf("%d", msg.Partition))
					span.Tag("event.type", evt.Type)
				}
			}
			// 调业务。Stage 94 PR-2a：handlerErr 透传给 span.EndSpan，让 OAP 标记失败
			var handlerErr error
			if handlerErr = h.Handler(sess.Context(), evt); handlerErr != nil {
				h.handleFailure(sess, msg, handlerErr, maxRetries)
			} else {
				// 业务成功：清空 attempts（key 复用 = 同事件再次成功）
				if key := attemptKey(msg); key != "" {
					h.attemptsMu.Lock()
					delete(h.attempts, key)
					h.attemptsMu.Unlock()
				}
				sess.MarkMessage(msg, "")
			}
			// Stage 94 PR-2a §P0-3：case 末尾立刻 EndSpan（不用 defer —— defer 绑定到
			// ConsumeClaim 函数返回，会让 N 条消息 span 累积到 consumer 退出才收尾）
			if span != nil {
				span.EndSpan(handlerErr)
			}
		case <-sess.Context().Done():
			return nil
		}
	}
}

// KafkaConsumer 实现见 consumer_runner.go（Round 4.7 §F 拆分）