// Package consumer — dlq_metrics.go
//
// Round 2.3 §PR-1: DLQ publish 计数器。
//
// 背景：kafka-pipeline-pending-decisions.md §P1-14 — DLQ 投递不计数、无 lag 指标，
// deploy/prometheus/rules/kafka-lag.yml:7-9 注释明写"DLQ 不告警——毒消息进 DLQ 是预期行为"。
//
// 但 DLQ.Publish 失败（broker 不可达、sarama 超时）时仅 slog.Error 即丢弃，业务消息已
// MarkMessage 也无法挽回——属于"业务 + DLQ 双失败"的不可观测黑洞。本 counter 让黑洞可见：
//
//   emotion_echo_dlq_publish_total{result="success"} — DLQ 投递成功累计
//   emotion_echo_dlq_publish_total{result="failure"} — DLQ 投递失败累计（broker 不可达 / 超时）
//
// 部署侧告警（deploy/prometheus/rules/kafka-dlq.yml，本 round 同步）：
//
//   rate(emotion_echo_dlq_publish_total{result="failure"}[5m]) > 0.1
//     for 5m  → severity=critical，page on-call
//
// 设计取舍：
//   - counter 在 caller (consumer.go) 调，而不是 DLQ.Publish 实现内部：
//     (a) 三个 DLQPublisher 实现 (Noop/InMemory/Kafka) 共享同一计数器，避免每个实现重复埋点
//     (b) 业务语义"DLQ 投递成败"由调用方决定——InMemory 失败 vs Kafka 失败统一为 failure
//   - analytics-svc 独立 counter（emotion_echo_analytics_dlq_publish_total）便于按 svc 拆 dashboard
package consumer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DLQPublishTotal DLQ 投递结果计数器
//
// label=result 取值 "success" | "failure"。
//
// 用法：
//
//	if dlqErr := h.DLQ.Publish(ctx, entry); dlqErr != nil {
//	    IncDLQPublishResult(false) // failure
//	    slog.Error(...)
//	} else {
//	    IncDLQPublishResult(true) // success
//	}
var DLQPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "emotion_echo_dlq_publish_total",
	Help: "Total DLQ publish attempts, labeled by result (success|failure). Failure means broker is unreachable or timed out; business message is already MarkMessage'd and lost.",
}, []string{"result"})

// IncDLQPublishResult 递增 DLQ 计数器
//
// success=true → result="success"，success=false → result="failure"。
func IncDLQPublishResult(success bool) {
	r := "success"
	if !success {
		r = "failure"
	}
	DLQPublishTotal.WithLabelValues(r).Inc()
}