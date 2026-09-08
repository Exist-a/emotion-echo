// Package fusion — Stage 35 · PR-6 GREEN + PR-OBS-11
//
// fusion_metrics.go 是 ai-svc 内部的 metrics 适配层（薄 wrapper）。
// 真正的 collector 定义在 shared/pkg/metrics/fusion_metrics.go（避免 shared → 业务包的反向依赖）。
//
// 适配函数（每个都是 1 行透传到 shared）：
//   - RecordLLMCall(outcome)
//   - ObserveLLMLatency(outcome, seconds)
//   - RecordFallback(stage)
//   - RecordWorkerTick(outcome)
//   - SetLRUStat(kind, value)
//   - RecordFusionCall(modality, result)         // PR-OBS-11
//   - ObserveFusionDuration(modality, seconds)   // PR-OBS-11
package fusion

import (
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
)

// RecordLLMCall LLM 调用计数。
func RecordLLMCall(outcome string) {
	sharedmetrics.FusionLLMCallTotal.WithLabelValues(outcome).Inc()
}

// ObserveLLMLatency LLM 调用耗时。
func ObserveLLMLatency(outcome string, seconds float64) {
	sharedmetrics.FusionLLMLatencySeconds.WithLabelValues(outcome).Observe(seconds)
}

// RecordFallback Fallback 计数。
func RecordFallback(stage string) {
	sharedmetrics.FusionFallbackTotal.WithLabelValues(stage).Inc()
}

// RecordWorkerTick Worker tick 计数。
func RecordWorkerTick(outcome string) {
	sharedmetrics.FusionWorkerTickTotal.WithLabelValues(outcome).Inc()
}

// SetLRUStat 更新 LRU gauge。
func SetLRUStat(kind string, value float64) {
	sharedmetrics.FusionLRUStat.WithLabelValues(kind).Set(value)
}

// RecordFusionCall PR-OBS-11: fusion 整体调用计数（跨多模态入口）
//
// modality: text/face/voice/fused
// result: success/failure/skipped_lru/circuit_open
func RecordFusionCall(modality, result string) {
	sharedmetrics.FusionCallsTotal.WithLabelValues(modality, result).Inc()
}

// ObserveFusionDuration PR-OBS-11: fusion 整体调用耗时 histogram
func ObserveFusionDuration(modality string, seconds float64) {
	sharedmetrics.FusionDurationSeconds.WithLabelValues(modality).Observe(seconds)
}