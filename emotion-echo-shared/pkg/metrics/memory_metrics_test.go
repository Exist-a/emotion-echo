// Package metrics — memory_metrics_test.go
//
// CL-08a · T-18：记忆指标的注册与 label 契约（防止改名/改 label 后悄悄消失）。

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMemoryExtractTotal_Registered(t *testing.T) {
	MemoryExtractTotal.WithLabelValues(MemoryResultOK).Inc()
	got, err := RegistryGatherCounter("emotion_echo_memory_extract_total", map[string]string{"result": MemoryResultOK})
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got < 1 {
		t.Fatalf("emotion_echo_memory_extract_total{result=ok} = %v, want >= 1", got)
	}
}

func TestMemoryFactsWrittenTotal_Registered(t *testing.T) {
	MemoryFactsWrittenTotal.Add(2)
	got, err := RegistryGatherCounter("emotion_echo_memory_facts_written_total", nil)
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got < 2 {
		t.Fatalf("emotion_echo_memory_facts_written_total = %v, want >= 2", got)
	}
}

func TestMemorySummaryBuildTotal_Registered(t *testing.T) {
	MemorySummaryBuildTotal.WithLabelValues(MemoryResultError).Inc()
	got, err := RegistryGatherCounter("emotion_echo_memory_summary_build_total", map[string]string{"result": MemoryResultError})
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if got < 1 {
		t.Fatalf("emotion_echo_memory_summary_build_total{result=error} = %v, want >= 1", got)
	}
}

func TestMemoryExtractDuration_Observed(t *testing.T) {
	labels := map[string]string{"result": MemoryResultOK}
	before := histogramCount(t, "emotion_echo_memory_extract_duration_seconds", labels)
	MemoryExtractDurationSeconds.WithLabelValues(MemoryResultOK).Observe(0.42)
	after := histogramCount(t, "emotion_echo_memory_extract_duration_seconds", labels)
	if after != before+1 {
		t.Fatalf("sample_count 应从 %d 增到 %d，实际 %d（只断言注册成功会漏掉 Observe 坏掉）", before, before+1, after)
	}
}

// histogramCount 读指定 histogram 的 sample_count（用于证明 Observe 真的记了数）。
func histogramCount(t *testing.T, name string, labels map[string]string) uint64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) && m.GetHistogram() != nil {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}
