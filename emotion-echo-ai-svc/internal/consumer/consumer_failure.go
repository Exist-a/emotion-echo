// Package consumer — Round 4.7 §F：consumer.go 拆分
//
// 本文件包含 consumer.go 抽出的失败处理逻辑（handleFailure / attemptKey /
// extractSw8Header）。从原 consumer.go 迁出，让 consumer.go 主文件 < 200 行
// （plan §六 Round 4.7 PR-2 目标）。
//
// 失败处理链路（Stage 30-C A2 + Round 5b §B + Round 2.3 §PR-1）：
//   - attempt < MaxRetries：递增计数，不 MarkMessage（sarama 自动重投）
//   - attempt >= MaxRetries：调 DLQ.Publish（若 DLQ 非 nil）+ MarkMessage + 重置计数
//   - DLQ 为 nil：退化为原行为（不 MarkMessage，保留向后兼容）
//   - DLQ 投递失败：IncDLQPublishResult(false) 计数黑洞（Round 2.3 §PR-1）
package consumer

import (
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
)

// handleFailure 处理 Handler 失败。
//
// Stage 30-C A2 失败语义：
//   - attempt < MaxRetries：递增计数，不 MarkMessage（sarama 自动重投）
//   - attempt >= MaxRetries：调 DLQ.Publish（若 DLQ 非 nil）+ MarkMessage + 重置计数
//   - DLQ 为 nil：退化为原行为（不 MarkMessage，保留向后兼容）
func (h *ConsumerGroupHandler) handleFailure(
	sess sarama.ConsumerGroupSession,
	msg *sarama.ConsumerMessage,
	handlerErr error,
	maxRetries int,
) {
	key := attemptKey(msg)
	// Round 5b §B: 读写 attempts 加 sync.Mutex 守卫(防御性,跨 goroutine 安全)
	h.attemptsMu.Lock()
	if h.attempts == nil {
		h.attempts = make(map[string]int)
	}
	h.attempts[key]++
	attempt := h.attempts[key]
	h.attemptsMu.Unlock()

	if attempt <= maxRetries {
		slog.Error("consumer handler err (will retry)",
			"attempt", attempt, "max_retries", maxRetries, "key", key, "err", handlerErr)
		return
	}

	// 已达最大重试 → DLQ + Mark
	if h.DLQ != nil {
		// P1-2 (Round 1): 把原消息 headers 透传到 DLQ，便于 OAP 上能追到 sw8/trace_id。
		// 否则 DLQ 消息在 OAP 上是孤儿，与上游 producer trace 断链。
		dlqHeaders := make(map[string]string, len(msg.Headers))
		for _, hh := range msg.Headers {
			dlqHeaders[string(hh.Key)] = string(hh.Value)
		}
		dlqEntry := DLQEntry{
			Topic:         msg.Topic,
			Key:           msg.Key,
			Value:         msg.Value,
			Attempts:      attempt,
			LastError:     handlerErr.Error(),
			OriginalTopic: msg.Topic,
			Headers:       dlqHeaders,
		}
		if dlqErr := h.DLQ.Publish(sess.Context(), dlqEntry); dlqErr != nil {
			// Round 2.3 §PR-1：DLQ 投递失败计数（kafka-pipeline-pending-decisions.md §P1-14）。
			// 业务消息已 MarkMessage 也无法挽回——属于"业务 + DLQ 双失败"黑洞，本 counter 让黑洞可见。
			IncDLQPublishResult(false)
			slog.ErrorContext(sess.Context(), "consumer DLQ publish failed (dropping msg)", "err", dlqErr)
		} else {
			IncDLQPublishResult(true)
		}
	}
	slog.ErrorContext(sess.Context(), "consumer handler err after retries → DLQ",
		"attempt", attempt, "key", key, "err", handlerErr)
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

// extractSw8Header 从 sarama RecordHeader 列表抽 sw8 header value
//
// Stage 92 PR-2: chat-svc producer (PR-1) 写到 Kafka header["sw8"] 的字符串
// 由此函数抽回 → 喂给 Tracer.CreateEntrySpan 的 extractor → go2sky 重建父 trace。
//
// header 名常量与 chat-svc kafka_publisher.sw8HeaderName 一致 ("sw8")；
// 这里不复用 shared 常量（避免引入 go2sky 依赖到 ai-svc 的 consumer 包）。
func extractSw8Header(headers []*sarama.RecordHeader) string {
	for _, h := range headers {
		if h == nil {
			continue
		}
		if string(h.Key) == "sw8" {
			return string(h.Value)
		}
	}
	return ""
}
