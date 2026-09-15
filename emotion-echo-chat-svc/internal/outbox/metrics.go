// Package outbox — metrics.go
//
// Stage 86（kafka-reliability-gaps.md §3.6 告警半场）：
// outbox 行进入 dead 状态时递增计数器，供 Prometheus 告警规则
// deploy/prometheus/rules/outbox-dead.yml 抓取（OutboxEventsDead, severity=critical）。
//
// dead = 发布重试 MaxAttempts 次仍失败、永久放弃的事件行——意味着事件丢失，
// 与 consumer lag（warning）不同级别，值班需要人工排查/回放。
//
// Stage 94 PR-3 §P0-5：outbox_sent_via_fallback_total — chat-svc main.go Kafka
// producer init 失败时 fallback InMemoryEventPublisher 时递增。这是 §P0-5
// "Kafka producer InMemory fallback 静默击穿 outbox" 的"短期 C"修复
// (kafka-pipeline-pending-decisions.md D1): 让黑洞变成可见
// ——指标非 0 时说明事件"sent 但未进 Kafka"正在发生,需立即排查。
// 长期 B（relay 收紧,kafka init 失败时 relay 不启动）独立 sprint 跟踪。
package outbox

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// OutboxEventsDeadTotal dead 行计数器（全局，无 label——dead 事件预期极低，不需要维度拆分）
var OutboxEventsDeadTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "emotion_echo_outbox_events_dead_total",
	Help: "Total number of outbox rows marked dead (publish retries exceeded MaxAttempts, events permanently dropped).",
})

// IncDead relay MarkDead 成功后调用
func IncDead() { OutboxEventsDeadTotal.Inc() }

// OutboxSentViaFallbackTotal Kafka init 失败 → fallback InMemory 计数器
// （Stage 94 PR-3 §P0-5 短期 C 修复）
//
// 含义:chat-svc 启动时若 NewKafkaEventPublisher 失败且 KAFKA_ENABLED=true,
// main.go fallback 到 InMemoryEventPublisher 同时 IncSentViaFallback() 一次。
// 后续每条 outbox 事件被 relay MarkSent 但实际未进 Kafka,此 counter **不再递增**
// （事件级黑洞不可逐条观测,只能用 Kafka 消息数 vs MarkSent 行数对账）。
//
// 推荐告警规则（部署 prometheus/rules/ 时补）:
//   - emotion_echo_outbox_sent_via_fallback_total > 0 持续 1m → page on-call
//     （说明生产模式下 Kafka 不可达,事件正在静默丢失,需立即排查）
var OutboxSentViaFallbackTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "emotion_echo_outbox_sent_via_fallback_total",
	Help: "Number of times chat-svc fell back to InMemoryEventPublisher due to Kafka init failure (each startup; events silently lost thereafter).",
})

// IncSentViaFallback chat-svc main.go Kafka init 失败 fallback 时调用
func IncSentViaFallback() { OutboxSentViaFallbackTotal.Inc() }

// OutboxCleanedTotal Round 2.1 §D2 cleanup job 计数器
//
// label=status 取值 "sent" | "dead"。
// 记录每次 CleanupOnce 单轮删除的行数（按 status 分桶）。
// 运维侧：rate > 0 持续说明 cleanup job 在工作；rate = 0 持续 24h 可能是 retention 配置过宽。
var OutboxCleanedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "emotion_echo_outbox_cleaned_total",
	Help: "Total outbox rows deleted by CleanupOnce, labeled by sent|dead.",
}, []string{"status"})

// IncCleaned cleanup 完成后调用
func IncCleaned(status string, n int64) {
	if n <= 0 {
		return
	}
	OutboxCleanedTotal.WithLabelValues(status).Add(float64(n))
}
