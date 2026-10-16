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
	"context"
	"log/slog"
	"time"

	"github.com/IBM/sarama"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	sharedmessaging "github.com/emotion-echo/shared/pkg/messaging"

	"emotion-echo-ai-svc/internal/events"
)

// deliverWithRetry 在**同一条消息**上原地重试 Handler，直到成功或预算耗尽。
//
// 预算 = maxRetries+1 次尝试（与旧 DLQ.Attempts=4 对齐）；相邻尝试按
// backoffFor(attempt) 退避。返回 (实际尝试次数, 最终 error)；成功时 error 为 nil。
func (h *ConsumerGroupHandler) deliverWithRetry(
	ctx context.Context,
	evt *events.Event,
	msg *sarama.ConsumerMessage,
	maxRetries int,
) (int, error) {
	var err error
	for attempt := 1; ; attempt++ {
		if err = h.Handler(ctx, evt); err == nil {
			return attempt, nil
		}
		if attempt > maxRetries {
			return attempt, err
		}
		d := h.backoffFor(attempt)
		slog.ErrorContext(ctx, "consumer handler err (will retry in-place)",
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
//
// 返回值（CL-03 Phase 4b / CL-D-16=A）：
//   - nil：消息已安全落到 DLQ（或被 Noop/未配置 DLQ "接受"）⇒ 调用方 Mark 前进
//   - err：DLQ 投递**真的失败** ⇒ 调用方**不得 Mark**，否则"业务 + DLQ 双失败 = 消息永久丢失"
//
// DLQ 为 nil（未配置，dev）时返回 nil：无处可投但也不能卡死消费（与改前一致）。
func (h *ConsumerGroupHandler) publishToDLQ(
	ctx context.Context,
	msg *sarama.ConsumerMessage,
	handlerErr error,
	attempts int,
) error {
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
		// CL-01 G4：DLQ 投递建 exit span（挂 kafka-consume entry span 下），并把 sw8
		// 注入 dlqHeaders（map 引用类型，随 dlqEntry 一并发出）⇒ DLQ 消息可续接同一条 trace。
		var dlqSpan grpcinterceptor.Span
		if h.Tracer != nil {
			injector := func(k, v string) error { dlqHeaders[k] = v; return nil }
			if _, s, derr := h.Tracer.CreateExitSpan(ctx, "kafka-publish-dlq", msg.Topic, injector); derr == nil {
				dlqSpan = s
			}
		}
		dlqErr := h.DLQ.Publish(ctx, dlqEntry)
		if dlqSpan != nil {
			dlqSpan.EndSpan(dlqErr)
		}
		if dlqErr != nil {
			// Round 2.3 §PR-1：DLQ 投递失败计数（kafka-pipeline-pending-decisions.md §P1-14）。
			//
			// CL-03 Phase 4b 语义修正：此处**不再**是"可观测的黑洞"——调用方拿到本 err 后
			// 会停止 Mark 并结束本次 claim，消息留待下次 session 重投（幂等由
			// emotion_analysis.event_id UNIQUE + ON CONFLICT DO NOTHING 保证）。
			IncDLQPublishResult(false)
			slog.ErrorContext(ctx, "consumer DLQ publish failed (will NOT mark; message stays for redelivery)", "err", dlqErr)
		} else {
			IncDLQPublishResult(true)
		}
		slog.ErrorContext(ctx, "consumer handler err after retries → DLQ",
			"attempts", attempts, "key", string(msg.Key), "err", handlerErr)
		return dlqErr
	}
	slog.ErrorContext(ctx, "consumer handler err after retries → DLQ",
		"attempts", attempts, "key", string(msg.Key), "err", handlerErr)
	return nil
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
