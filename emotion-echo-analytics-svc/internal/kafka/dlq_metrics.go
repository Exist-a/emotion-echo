// Package kafka — dlq_metrics.go
//
// Round 2.3 §PR-1 analytics-svc 镜像 ai-svc/dlq_metrics.go，metric 名按 svc 拆：
//   - ai-svc:       emotion_echo_dlq_publish_total
//   - analytics-svc: emotion_echo_analytics_dlq_publish_total
//
// 理由：dashboard / alert 按 svc 维度拆，避免 ai-svc DLQ 故障被 analytics-svc 正常
// 流量掩盖；按 service label 分桶在 prometheus 侧成本更高且不支持记录 cardinality。
//
// 共享调用规约：consumer.go:305 调 h.dlq.Publish 之后立即 IncDLQPublishResult。
// 删 caller 埋点 → analytics 黑洞不可观测（与 ai-svc dlq_metrics_test.go 同款 caller-wiring 测试）。
package kafka

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// DLQPublishTotal analytics-svc DLQ 投递结果计数器
var DLQPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "emotion_echo_analytics_dlq_publish_total",
	Help: "Total analytics-svc DLQ publish attempts, labeled by result (success|failure).",
}, []string{"result"})

// IncDLQPublishResult 递增 DLQ 计数器
func IncDLQPublishResult(success bool) {
	r := "success"
	if !success {
		r = "failure"
	}
	DLQPublishTotal.WithLabelValues(r).Inc()
}