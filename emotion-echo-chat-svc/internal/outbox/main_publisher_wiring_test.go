package outbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== CL-03 Phase 5 接线守卫（CL-D-19=C / CL-F-15）=====
//
// 为什么用 source 断言：main.go 的装配逻辑（Kafka 初始化失败后走哪条路）无法在单测里
// 真跑（要真 Kafka + 真 DB）。仓内同型先例：cleanup_test.go 的
// TestCleanupOnce_CallerWiringInMainGo、internal/logic 的 outbox_trace_sw8_wiring_test.go。
// 运行时行为由 Phase 7 的"坏 broker 起栈"验证覆盖（见阶段 report）。
//
// 本测试防的回归：有人把 init 失败分支改回 `pub = NewInMemoryEventPublisher()`
// （那正是 CL-F-15 的黑洞：relay 照常 MarkSent，事件只进内存）。

func mainGoSource(t *testing.T) string {
	t.Helper()
	bs, err := os.ReadFile(filepath.Join("..", "..", "main.go"))
	require.NoError(t, err, "读不到 chat-svc/main.go")
	return string(bs)
}

func TestMainGo_KafkaInitFailure_DoesNotFallBackToInMemory(t *testing.T) {
	src := mainGoSource(t)

	assert.Contains(t, src, "events.NewUnavailablePublisher(err)",
		"Kafka 初始化失败时必须装 UnavailablePublisher（Publish 报错 ⇒ relay 不 MarkSent）")
	assert.Contains(t, src, "relay.PublisherFn = pubHolder.Get",
		"relay 必须每轮现读 publisher（后台重试成功后热替换才能生效）")
	assert.Contains(t, src, "StartKafkaPublisherBackgroundRetry",
		"必须启动后台重试（对齐 CL-F-14 的自愈思路），否则 init 失败就永久不可用")

	// 反例守卫：Kafka 分支里不得再出现"改投内存"的注释/调用组合
	kafkaIdx := strings.Index(src, "if kafkaEnabled {")
	require.GreaterOrEqual(t, kafkaIdx, 0, "main.go 里找不到 kafkaEnabled 分支")
	rest := src[kafkaIdx:]
	if end := strings.Index(rest, "\n\t} else if db != nil {"); end > 0 {
		rest = rest[:end]
	}
	assert.NotContains(t, rest, "NewInMemoryEventPublisher()",
		"Kafka 分支内**不得**再退回内存 publisher —— 那是 CL-F-15 的静默丢事件黑洞")
}
