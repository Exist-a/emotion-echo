// Package metrics — memory_metrics.go
//
// CL-08a · T-18：跨会话记忆的抽取/摘要指标。
//
// 纪律（CL-01）：**埋了就必须有人看** —— 这四个 collector 在
// `deploy/grafana/dashboards/emotion-echo-business.json` 有对应面板，
// 并由 `deploy/obs_monitoring.test.js` 组 ⑨ 双向钉住（代码名 ↔ 面板 expr）。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 抽取/摘要的 result label 合法值。
const (
	MemoryResultOK      = "ok"
	MemoryResultError   = "error"
	MemoryResultSkipped = "skipped"
)

// MemoryExtractTotal 事实抽取次数（message / note 两源合计）。
var MemoryExtractTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_memory_extract_total",
		Help: "Total number of memory extraction runs, labeled by result (ok|error|skipped).",
	},
	[]string{"result"},
)

// MemoryExtractDurationSeconds 单次抽取耗时（含 LLM 调用）。
var MemoryExtractDurationSeconds = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "emotion_echo_memory_extract_duration_seconds",
		Help:    "Histogram of memory extraction latency.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 4, 8, 15},
	},
	[]string{"result"},
)

// MemoryFactsWrittenTotal 新写入的事实行数（幂等跳过的不计）。
var MemoryFactsWrittenTotal = promauto.NewCounter(
	prometheus.CounterOpts{
		Name: "emotion_echo_memory_facts_written_total",
		Help: "Total number of memory facts inserted (idempotent skips excluded).",
	},
)

// MemorySummaryBuildTotal 用户级摘要重建次数。
var MemorySummaryBuildTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_memory_summary_build_total",
		Help: "Total number of user memory summary rebuilds, labeled by result (ok|error).",
	},
	[]string{"result"},
)
