// Package outbox — metrics_test.go
//
// Stage 94 PR-3 §P0-5 测试: 验证 outbox_sent_via_fallback_total counter 存在
// 且 IncSentViaFallback() 能递增（被 chat-svc main.go Kafka init 失败 fallback
// 路径调用）。字面量断言钉死 chat-svc main.go 必须调用此 helper 而非默默
// fallback（kafka-pipeline-pending-decisions.md D1 §P0-5 短期 C 修复）。
package outbox

import (
	"os"
	"strings"
	"testing"

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"

	"github.com/stretchr/testify/require"
)

// fallbackCounter 取全局 fallback 计数器当前值
func fallbackCounter(t *testing.T) float64 {
	t.Helper()
	v, err := sharedmetrics.RegistryGatherCounter("emotion_echo_outbox_sent_via_fallback_total", nil)
	require.NoError(t, err)
	return v
}

// TestMetrics_SentViaFallback_Increments §P0-5 RED: 验证 counter 存在 + 递增
func TestMetrics_SentViaFallback_Increments(t *testing.T) {
	before := fallbackCounter(t)
	IncSentViaFallback()
	IncSentViaFallback()
	IncSentViaFallback()
	after := fallbackCounter(t)
	require.Equal(t, before+3, after, "IncSentViaFallback 应递增 counter 3 次")
}

// TestMetrics_SentViaFallback_ChatSvcMainGo_UsesHelper §P0-5 字面量断言:
//
// chat-svc main.go 在 Kafka init 失败 fallback InMemoryEventPublisher 时
// 必须显式调用 outbox.IncSentViaFallback(),而不是仅打 log 一行就静默继续。
// (原 §P0-5 bug: line 151 仅 log "[kafka] producer init failed: ... (fallback to in-memory)"
// 后继续 — 黑洞事件)
//
// 与 TestRelay_MaxAttemptsReached_MarksDead 同模式: 用源码 grep 钉死
// chat-svc main.go 调了 outbox.IncSentViaFallback()。
func TestMetrics_SentViaFallback_ChatSvcMainGo_UsesHelper(t *testing.T) {
	mainBytes, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Skipf("cannot read chat-svc main.go: %v", err)
	}
	src := string(mainBytes)

	if !strings.Contains(src, `outbox.IncSentViaFallback()`) {
		t.Error("chat-svc main.go 缺 outbox.IncSentViaFallback() 调用\n" +
			"§P0-5 修复要求:Kafka init 失败 fallback InMemoryEventPublisher 时\n" +
			"必须显式递增 counter(让黑洞可见),而不是仅 log 一行就继续")
	}

	// 反向断言:不应直接打 fallback log 后无 counter(原 §P0-5 bug 模式)
	// —— 防止后续"加了 IncSentViaFallback 但被删"漂移
	if !strings.Contains(src, `fallback to in-memory`) {
		t.Error("chat-svc main.go 缺 'fallback to in-memory' log —— §P0-5 修复要求保留\n" +
			"log + counter(双重信号)而非单一信号")
	}
}