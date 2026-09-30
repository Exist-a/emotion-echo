package fusion

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 #31：熔断器的两个阈值可热更。
//
// 这两个是"故障处置时最想立刻调"的参数：
//   - LLM 抖动但服务没挂 ⇒ 误触发熔断 ⇒ 要**调高** FailThreshold
//   - Closed 卡死不跳闸 ⇒ 要**调低**
//   - Open 状态想快点试探恢复 ⇒ 要**调小** OpenSeconds
//
// 此前它们在构造期存入普通字段（llm_breaker.go:83-84），运行期不可变。

func TestCircuitBreaker_FailThresholdHotReload(t *testing.T) {
	t.Parallel()

	var threshold atomic.Int64
	threshold.Store(5)

	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 5, OpenSeconds: 30 * time.Second})
	b.FailThresholdFn = func() int { return int(threshold.Load()) }

	// 失败 3 次：阈值 5 时不应熔断
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	require.Equal(t, BreakerClosed, b.State(), "3 次失败 < 阈值 5，应仍 Closed")

	// 热更阈值到 2。
	//
	// 语义澄清：熔断器的状态翻转是**事件驱动**的（在 RecordFailure 内评估），
	// 不是"读配置时立刻重算" —— 热更阈值不会让已 Closed 的实例凭空跳闸，
	// 而是在**下一次失败**时按新阈值判定。这是合理设计（避免配置变更引入
	// 无来源的状态跃迁），测试须按此语义断言。
	threshold.Store(2)
	assert.Equal(t, BreakerClosed, b.State(),
		"仅改阈值不应让状态凭空跃迁（事件驱动语义）")

	// 再记一次失败：累计 4 次 ≥ 新阈值 2 ⇒ 应转 Open
	b.RecordFailure()
	assert.Equal(t, BreakerOpen, b.State(),
		"阈值热更到 2 后再次失败应转 Open —— 证明判定读的是新值而非构造期的 5")
}

func TestCircuitBreaker_OpenSecondsHotReload(t *testing.T) {
	t.Parallel()

	var openSecs atomic.Int64
	openSecs.Store(int64(3600 * time.Second)) // 先设很长

	now := time.Now()
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: 3600 * time.Second})
	b.OpenSecondsFn = func() time.Duration { return time.Duration(openSecs.Load()) }
	b.nowFunc = func() time.Time { return now }

	// 触发熔断
	_ = b.Allow()
	b.RecordFailure()
	require.Equal(t, BreakerOpen, b.State())

	// 时间前进 1 分钟：3600s 的 Open 期未到，仍 Open
	now = now.Add(1 * time.Minute)
	b.transitionIfNeeded()
	assert.Equal(t, BreakerOpen, b.State(), "OpenSeconds=3600 时 1 分钟后应仍 Open")

	// 热更 OpenSeconds 到 30s ⇒ 已过 1 分钟 ⇒ 应转 HalfOpen
	openSecs.Store(int64(30 * time.Second))
	b.transitionIfNeeded()
	assert.Equal(t, BreakerHalfOpen, b.State(),
		"OpenSeconds 热更到 30s 后应转 HalfOpen —— 证明判定读的是新值（而非构造期的 3600s）")
}

// TestCircuitBreaker_NilHooks_KeepConstructorValues 向后兼容。
func TestCircuitBreaker_NilHooks_KeepConstructorValues(t *testing.T) {
	t.Parallel()

	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 7, OpenSeconds: 45 * time.Second})
	assert.Equal(t, 7, b.effectiveFailThreshold(), "未装钩子时应返回构造期值")
	assert.Equal(t, 45*time.Second, b.effectiveOpenSeconds())

	// 非法钩子返回值必须被忽略（回退构造期值）
	b.FailThresholdFn = func() int { return 0 }
	b.OpenSecondsFn = func() time.Duration { return 0 }
	assert.Equal(t, 7, b.effectiveFailThreshold(), "0 阈值必须被忽略")
	assert.Equal(t, 45*time.Second, b.effectiveOpenSeconds(), "0 时长必须被忽略")
}
