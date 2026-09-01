// Package metrics — Stage 35 · PR-6 RED
//
// Fusion-specific Prometheus metrics collector。
// 与 emotion-echo-ai-svc 内部 fusion 包解耦：本文件不 import fusion，
// 避免 shared → 业务包的反向依赖（5 个 svc 都会 link shared）。
//
// 4 个 collector（ADR-15 §F）：
//   - FusionLLMCallTotal:        CounterVec{outcome: success|json_parse_err|timeout|http_5xx|invalid_output|circuit_open|other}
//   - FusionLLMLatencySeconds:   HistogramVec{}
//   - FusionFallbackTotal:       CounterVec{stage: llm_to_late|late_to_skip}
//   - FusionWorkerTickTotal:     CounterVec{outcome: ok|error|skipped_lru}
//   - FusionLRUStat:             GaugeVec{kind: size|hits|misses}
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// LLMCall outcome label 合法值。
const (
	LLMOutcomeSuccess       = "success"
	LLMOutcomeJSONParseErr  = "json_parse_err"
	LLMOutcomeTimeout       = "timeout"
	LLMOutcomeHTTP5xx       = "http_5xx"
	LLMOutcomeInvalidOutput = "invalid_output"
	LLMOutcomeCircuitOpen   = "circuit_open"
	LLMOutcomeOther         = "other"
)

// Fallback stage label 合法值。
const (
	FallbackStageLLMToLate = "llm_to_late"
	FallbackStageLateToSkip = "late_to_skip"
)

// WorkerTick outcome label 合法值。
const (
	WorkerTickOK         = "ok"
	WorkerTickError      = "error"
	WorkerTickSkippedLRU = "skipped_lru"
)

// FusionLLMCallTotal LLM 调用计数。
var FusionLLMCallTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_fusion_llm_call_total",
		Help: "Total number of LLM fusion calls, labeled by outcome.",
	},
	[]string{"outcome"},
)

// FusionLLMLatencySeconds LLM 调用耗时 histogram。
var FusionLLMLatencySeconds = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "emotion_echo_fusion_llm_latency_seconds",
		Help:    "Histogram of LLM fusion call latency.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	},
	[]string{"outcome"},
)

// FusionFallbackTotal Fallback 计数（stage 区分）。
var FusionFallbackTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_fusion_fallback_total",
		Help: "Total number of fallback events, labeled by stage.",
	},
	[]string{"stage"},
)

// FusionWorkerTickTotal Worker tick 计数。
var FusionWorkerTickTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_fusion_worker_tick_total",
		Help: "Total number of FusionWorker tick outcomes.",
	},
	[]string{"outcome"},
)

// FusionLRUStat LRU 状态 gauge（size / hits / misses）。
var FusionLRUStat = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "emotion_echo_fusion_lru_stat",
		Help: "Fusion LRU state (size, hits, misses).",
	},
	[]string{"kind"},
)