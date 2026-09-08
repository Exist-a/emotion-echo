// Package fusion — Stage 35 · PR-6 RED
//
// fusion_metrics.go 是 ai-svc 内部的 metrics 适配层（薄 wrapper）。
// 真正的 collector 定义在 shared/pkg/metrics/fusion_metrics.go（避免循环依赖）。
//
// 4 个包装函数：
//   - RecordLLMCall(outcome string)
//   - ObserveLLMLatency(outcome string, seconds float64)
//   - RecordFallback(stage string)
//   - RecordWorkerTick(outcome string)
package fusion

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
)

// TestFusionMetrics_RecordLLMCall Counter 增量能被读取。
//
// 注：promauto 注册到 default registry，全局共享。多次测试可能影响其他 counter，
// 故只断言 delta > 0 而非精确值。
func TestFusionMetrics_RecordLLMCall(t *testing.T) {
	t.Parallel()
	before := readFusionCounter(t, "emotion_echo_fusion_llm_call_total",
		map[string]string{"outcome": sharedmetrics.LLMOutcomeSuccess})
	RecordLLMCall(sharedmetrics.LLMOutcomeSuccess)
	after := readFusionCounter(t, "emotion_echo_fusion_llm_call_total",
		map[string]string{"outcome": sharedmetrics.LLMOutcomeSuccess})
	assert.Greater(t, after, before, "counter should increase")
}

// TestFusionMetrics_RecordFallback Fallback counter 增量能被读取。
func TestFusionMetrics_RecordFallback(t *testing.T) {
	t.Parallel()
	before := readFusionCounter(t, "emotion_echo_fusion_fallback_total",
		map[string]string{"stage": sharedmetrics.FallbackStageLLMToLate})
	RecordFallback(sharedmetrics.FallbackStageLLMToLate)
	after := readFusionCounter(t, "emotion_echo_fusion_fallback_total",
		map[string]string{"stage": sharedmetrics.FallbackStageLLMToLate})
	assert.Greater(t, after, before)
}

// TestFusionMetrics_RecordWorkerTick Worker tick counter 增量能被读取。
func TestFusionMetrics_RecordWorkerTick(t *testing.T) {
	t.Parallel()
	before := readFusionCounter(t, "emotion_echo_fusion_worker_tick_total",
		map[string]string{"outcome": sharedmetrics.WorkerTickOK})
	RecordWorkerTick(sharedmetrics.WorkerTickOK)
	after := readFusionCounter(t, "emotion_echo_fusion_worker_tick_total",
		map[string]string{"outcome": sharedmetrics.WorkerTickOK})
	assert.Greater(t, after, before)
}

// ===== PR-OBS-11 RED: 2 case — fusion 整体调用 metrics =====
//
// 目的: PR-OBS-11 设计 (observability-sprint-b.md §三.11):
// 1. 触发 fusion → emotion_fusion_calls_total{modality, result} 增 1
// 2. emotion_fusion_duration_seconds histogram series 存在 (registry 已注册)
//
// 注: 与 Stage 35 已落 metric 区分:
//   - 现有 emotion_fusion_llm_call_total: LLM 调用计数 (specific stage)
//   - 新 emotion_fusion_calls_total: fusion 整体调用计数 (跨多模态入口)
//
// TDD: RED 阶段引用未实现的 RecordFusionCall + ObserveFusionDuration,
// 应编译失败 (undefined symbol)。

func TestFusionMetrics_RecordFusionCall(t *testing.T) {
	t.Parallel()
	const modality = "text"
	const result = "success"
	before := readFusionCounter(t, "emotion_fusion_calls_total",
		map[string]string{"modality": modality, "result": result})
	RecordFusionCall(modality, result)
	after := readFusionCounter(t, "emotion_fusion_calls_total",
		map[string]string{"modality": modality, "result": result})
	assert.Greater(t, after, before,
		"emotion_fusion_calls_total{modality=%s,result=%s} should increase", modality, result)
}

func TestFusionMetrics_DurationSecondsHistogramRegistered(t *testing.T) {
	// 触发一次 fusion 让 histogram 出现至少一个 sample
	ObserveFusionDuration("text", 0.123)

	// 检查 registry 里有 emotion_fusion_duration_seconds series
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "emotion_fusion_duration_seconds" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("emotion_fusion_duration_seconds histogram should be registered to default registry")
	}
}

// readFusionCounter 辅助函数：从 prometheus default registry 读指定 metric + labels 的当前值。
func readFusionCounter(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	m, err := sharedmetrics.RegistryGatherCounter(name, labels)
	if err != nil {
		t.Fatalf("read counter %s: %v", name, err)
	}
	return m
}