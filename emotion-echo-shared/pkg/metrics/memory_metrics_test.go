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
	MemoryExtractDurationSeconds.WithLabelValues(MemoryResultOK).Observe(0.42)
	// histogram 不通过 GetCounter 读取；这里只断言 Observe 不 panic 且指标名可收集。
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "emotion_echo_memory_extract_duration_seconds" {
			found = true
		}
	}
	if !found {
		t.Fatal("emotion_echo_memory_extract_duration_seconds 未注册")
	}
}
