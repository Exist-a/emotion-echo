// Package kafka — dlq_metrics_test.go
//
// Round 2.3 §PR-1 analytics-svc 镜像 ai-svc/dlq_metrics_test.go，断言契约一致：
//   - IncDLQPublishResult(true)  → emotion_echo_analytics_dlq_publish_total{result="success"} +1
//   - IncDLQPublishResult(false) → emotion_echo_analytics_dlq_publish_total{result="failure"} +1
//   - caller (consumer.go:305) 必须调 IncDLQPublishResult（红线契约）
package kafka

import (
	"os"
	"testing"

	"github.com/emotion-echo/shared/pkg/metrics"
	"github.com/stretchr/testify/require"
)

func gatherCounterVal(t *testing.T, label string) float64 {
	t.Helper()
	val, err := metrics.RegistryGatherCounter("emotion_echo_analytics_dlq_publish_total", map[string]string{"result": label})
	require.NoError(t, err)
	return val
}

func TestIncDLQPublishResult_SuccessIncrementsSuccessCounter(t *testing.T) {
	before := gatherCounterVal(t, "success")
	IncDLQPublishResult(true)
	require.Equal(t, before+1, gatherCounterVal(t, "success"),
		"IncDLQPublishResult(true) 应让 result=success counter +1")
}

func TestIncDLQPublishResult_FailureIncrementsFailureCounter(t *testing.T) {
	before := gatherCounterVal(t, "failure")
	IncDLQPublishResult(false)
	require.Equal(t, before+1, gatherCounterVal(t, "failure"),
		"IncDLQPublishResult(false) 应让 result=failure counter +1")
}

func TestDLQMetrics_CallerWiring(t *testing.T) {
	source, err := os.ReadFile("consumer.go")
	require.NoError(t, err)
	src := string(source)
	require.Contains(t, src, "h.dlq.Publish",
		"consumer.go 必须存在 h.dlq.Publish 调用点")
	require.Contains(t, src, "IncDLQPublishResult",
		"consumer.go 必须存在 IncDLQPublishResult 调用（caller 埋点），否则 analytics DLQ 失败黑洞不可观测")
}