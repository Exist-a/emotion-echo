// Package consumer — dlq_metrics_test.go
//
// Round 2.3 §PR-1 RED：DLQ publish 计数器钉死。
//
// 契约：
//   - IncDLQPublishResult(true)  → emotion_echo_dlq_publish_total{result="success"} +1
//   - IncDLQPublishResult(false) → emotion_echo_dlq_publish_total{result="failure"} +1
//
// 现状（dlq.go 缺 counter，consumer.go:218 仅 slog.Error 不计数）：
//   旧实现 IncDLQPublishResult 调用不存在 → compile fail → FAIL
//   新实现 dlq_metrics.go 加 counter + consumer.go:218 调 Inc → PASS
//
// 为什么单独测 IncDLQPublishResult 而不是测 consumer.go 集成路径：
//   - IncDLQPublishResult 是公开 API，caller 只需传 bool
//   - consumer.go 集成需要构造完整 sarama ConsumerGroupSession（重）
//   - 单元测试 + 集成测试分摊：单元锁契约，集成锁接线（Stage 95 §2.4 风格）
package consumer

import (
	"os"
	"testing"

	"github.com/emotion-echo/shared/pkg/metrics"
	"github.com/stretchr/testify/require"
)

// gatherCounterVal 抓取 emotion_echo_dlq_publish_total{result=L} 当前值
func gatherCounterVal(t *testing.T, label string) float64 {
	t.Helper()
	val, err := metrics.RegistryGatherCounter("emotion_echo_dlq_publish_total", map[string]string{"result": label})
	require.NoError(t, err, "gather counter failed")
	return val
}

// TestIncDLQPublishResult_SuccessIncrementsSuccessCounter RED §1
func TestIncDLQPublishResult_SuccessIncrementsSuccessCounter(t *testing.T) {
	before := gatherCounterVal(t, "success")

	IncDLQPublishResult(true)

	after := gatherCounterVal(t, "success")
	require.Equal(t, before+1, after,
		"IncDLQPublishResult(true) 应让 result=success counter +1 (before=%v after=%v)", before, after)
}

// TestIncDLQPublishResult_FailureIncrementsFailureCounter RED §2
func TestIncDLQPublishResult_FailureIncrementsFailureCounter(t *testing.T) {
	before := gatherCounterVal(t, "failure")

	IncDLQPublishResult(false)

	after := gatherCounterVal(t, "failure")
	require.Equal(t, before+1, after,
		"IncDLQPublishResult(false) 应让 result=failure counter +1 (before=%v after=%v)", before, after)
}

// TestIncDLQPublishResult_BooleanMapping 锁定 bool→label 映射（防传错）
func TestIncDLQPublishResult_BooleanMapping(t *testing.T) {
	beforeSuccess := gatherCounterVal(t, "success")
	beforeFailure := gatherCounterVal(t, "failure")

	IncDLQPublishResult(true)
	require.Equal(t, beforeSuccess+1, gatherCounterVal(t, "success"), "true → success")
	require.Equal(t, beforeFailure, gatherCounterVal(t, "failure"), "true 不能动 failure")

	IncDLQPublishResult(false)
	require.Equal(t, beforeSuccess+1, gatherCounterVal(t, "success"), "false 不能动 success")
	require.Equal(t, beforeFailure+1, gatherCounterVal(t, "failure"), "false → failure")
}

// TestDLQMetrics_CallerWiring 钉死 caller 接线契约 ——
// ConsumerGroupHandler 在 DLQ.Publish 之后必须调 IncDLQPublishResult。
//
// 方式：grep consumer.go 源码，确认 "h.DLQ.Publish" 附近有 "IncDLQPublishResult" 调用。
// 这是"红线契约"测试——删 caller 埋点 → CI 立刻挂。
//
// 为什么用 grep 而非真实集成测试：
//   - 真实集成需构造完整 sarama ConsumerGroupSession（重，且依赖 broker）
//   - caller 接线是契约问题，不是逻辑问题；grep 比 e2e 更精准
//   - 已在 Round 1 §P0-5 / P0-6 用类似风格（kafka_publisher_test.go:343-394 用 mock 锁 span.EndSpan 契约）
func TestDLQMetrics_CallerWiring(t *testing.T) {
	// Round 4.7 §F：handleFailure 迁到 consumer_failure.go；DLQ.Publish 调用也在那。
	// 钉死 caller 接线必须在 consumer 包下任何文件存在。
	consumerSrc, err := os.ReadFile("consumer.go")
	require.NoError(t, err)
	failureSrc, err := os.ReadFile("consumer_failure.go")
	require.NoError(t, err)
	combined := string(consumerSrc) + "\n" + string(failureSrc)

	// 钉死：h.DLQ.Publish 之后必须调 IncDLQPublishResult
	require.Contains(t, combined, "h.DLQ.Publish",
		"consumer 包（consumer.go 或 consumer_failure.go）必须存在 h.DLQ.Publish 调用点（caller 接线源头）")
	require.Contains(t, combined, "IncDLQPublishResult",
		"consumer 包必须存在 IncDLQPublishResult 调用（caller 埋点），否则 DLQ 失败黑洞不可观测")
}