// Package outbox — metrics.go
//
// Stage 86（kafka-reliability-gaps.md §3.6 告警半场）：
// outbox 行进入 dead 状态时递增计数器，供 Prometheus 告警规则
// deploy/prometheus/rules/outbox-dead.yml 抓取（OutboxEventsDead, severity=critical）。
//
// dead = 发布重试 MaxAttempts 次仍失败、永久放弃的事件行——意味着事件丢失，
// 与 consumer lag（warning）不同级别，值班需要人工排查/回放。
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
