// Package consumer — Round 4.7 §F：consumer.go 拆分
//
// 本文件包含 consumer.go 抽出的失败处理逻辑（deliverWithRetry / publishToDLQ /
// backoffFor / extractSw8Header）。从原 consumer.go 迁出，让 consumer.go 主文件 < 200 行
// （plan §六 Round 4.7 PR-2 目标）。
//
// 失败处理链路（E2E-F-174 重写，2026-10-01；旧 handleFailure/attemptKey 删除）：
//   - Handler 失败 → **同一消息原地重试**（预算 = MaxRetries+1 次尝试，指数退避）
//   - 预算耗尽 → 调 DLQ.Publish（若非 nil）+ MarkMessage 前进
//   - DLQ 投递失败：IncDLQPublishResult(false) 计数黑洞（Round 2.3 §PR-1）
//
// 旧语义缺陷（账本 E2E-F-174 / D3 实测）：handleFailure 只递增计数不重投消息，
// "重试"靠后续同 key 消息失败次数推进 ⇒ 单条毒消息永不重新处理、可永久阻塞分区。
package consumer

import (
	"log/slog"
	"time"

	"github.com/IBM/sarama"

	sharedmessaging "github.com/emotion-echo/shared/pkg/messaging"

	"emotion-echo-ai-svc/internal/events"
)

// deliverWithRetry 在**同一条消息**上原地重试 Handler，直到成功或预算耗尽。
//
// 预算 = maxRetries+1 次尝试（与旧 DLQ.Attempts=4 对齐）；相邻尝试按
// backoffFor(attempt) 退避。返回 (实际尝试次数, 最终 error)；成功时 error 为 nil。
func (h *ConsumerGroupHandler) deliverWithRetry(
	sess sarama.ConsumerGroupSession,
	evt *events.Event,
	msg *sarama.ConsumerMessage,
	maxRetries int,
) (int, error) {
	var err error
	for attempt := 1; ; attempt++ {
		if err = h.Handler(sess.Context(), evt); err == nil {
			return attempt, nil
		}
		if attempt > maxRetries {
			return attempt, err
		}
		d := h.backoffFor(attempt)
		slog.ErrorContext(sess.Context(), "consumer handler err (will retry in-place)",
			"attempt", attempt, "max_retries", maxRetries, "key", string(msg.Key), "backoff", d, "err", err)
		time.Sleep(d)
	}
}

// backoffFor 第 attempt 次失败后的等待时长：注入的 BackoffFn 优先；
// 默认指数退避 min(2^attempt 秒, 30s)——2s/4s/8s…，给下游（分析模型/网络）恢复窗口。
func (h *ConsumerGroupHandler) backoffFor(attempt int) time.Duration {
	if h.BackoffFn != nil {
		return h.BackoffFn(attempt)
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// publishToDLQ 重试预算耗尽后的兜底：投 DLQ（带诊断 headers + sw8 透传）+ 计数。
// DLQ 为 nil 时只打日志（调用方无论如何都会 Mark 前进，毒消息不卡分区）。
func (h *ConsumerGroupHandler) publishToDLQ(
	sess sarama.ConsumerGroupSession,
	msg *sarama.ConsumerMessage,
	handlerErr error,
	attempts int,
) {
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
			Attempts:      attempts,
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
		"attempts", attempts, "key", string(msg.Key), "err", handlerErr)
}

// attemptKey 已删除（E2E-F-174）：旧"按 msg.Key 计数"语义失真，见文件头。
// extractSw8Header 从 sarama RecordHeader 列表抽 sw8 header value
//
// Stage 92 PR-2: chat-svc producer (PR-1) 写到 Kafka header["sw8"] 的字符串
// 由此函数抽回 → 喂给 Tracer.CreateEntrySpan 的 extractor → go2sky 重建父 trace。
//
// Round B: 收敛到 shared/pkg/messaging.ExtractSw8Header（kafka-pipeline
// D8-2 登记的"双份未收敛"项）。ai-svc 内的薄包装保留——本地调用方不用改 import。
func extractSw8Header(headers []*sarama.RecordHeader) string {
	return sharedmessaging.ExtractSw8Header(headers)
}

