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

// readFusionCounter 辅助函数：从 prometheus default registry 读指定 metric + labels 的当前值。
func readFusionCounter(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	m, err := sharedmetrics.RegistryGatherCounter(name, labels)
	if err != nil {
		t.Fatalf("read counter %s: %v", name, err)
	}
	return m
}