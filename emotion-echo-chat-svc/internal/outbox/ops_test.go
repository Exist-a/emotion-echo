package outbox

import (
	"sync"
	"testing"

	"emotion-echo-chat-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 E 组（测试点 #30）· D-32：chat-svc 的 4 个 Outbox 运营参数可热更。
//
// 为什么需要独立的容器（而不是直接改 c.Outbox.X）：
//
//	main.go 的清理协程在 `time.NewTicker(...)` 的闭包里读
//	`c.Outbox.SentRetentionDays` / `DeadRetentionDays`（main.go:248），
//	而 `c` 是 main() 的**局部变量**。若 Nacos 回调直接写 `c.Outbox.*`，
//	就是"一个 goroutine 写、另一个 goroutine 读"的**真实 data race**
//	（E2E-21 教训：-race 才抓得到，单测全绿也照样是 bug）。
//
//	故引入 Ops 容器：Nacos 回调只写它，业务协程只读它，中间用
//	sync/atomic 隔离。本测试锁定三件事：
//	  1. Apply 后读到的必须是新值（否则"推了不生效"）
//	  2. 0 值被忽略（保留原值）—— 避免误推 0 把死信阈值清零
//	  3. 并发 Apply + 读不产生数据竞争（须在 CI 的 -race 下复核）

func TestOps_ApplyUpdatesValues(t *testing.T) {
	t.Parallel()

	ops := NewOps(OpsConfig{
		MaxAttempts:       100,
		SentRetentionDays: 7,
		DeadRetentionDays: 30,
		CleanupIntervalS:  3600,
	})

	got := ops.Snapshot()
	assert.Equal(t, 100, got.MaxAttempts)
	assert.Equal(t, 7, got.SentRetentionDays)
	assert.Equal(t, 30, got.DeadRetentionDays)
	assert.Equal(t, 3600, got.CleanupIntervalS)

	// 模拟 Nacos 推送新值
	ops.Apply(OpsConfig{MaxAttempts: 5, SentRetentionDays: 1})

	got = ops.Snapshot()
	assert.Equal(t, 5, got.MaxAttempts, "热更后 MaxAttempts 必须是新值")
	assert.Equal(t, 1, got.SentRetentionDays, "热更后 SentRetentionDays 必须是新值")
	assert.Equal(t, 30, got.DeadRetentionDays, "未提供的字段应保留原值，不得被清零")
}

func TestOps_ApplyIgnoresZero(t *testing.T) {
	t.Parallel()

	ops := NewOps(OpsConfig{MaxAttempts: 100, SentRetentionDays: 7})

	// 运维误推 0（或 dataId 存在但字段为空）不应把阈值清零：
	// MaxAttempts=0 会关闭 dead 状态机，SentRetentionDays=0 会立即清库。
	ops.Apply(OpsConfig{MaxAttempts: 0, SentRetentionDays: 0, DeadRetentionDays: 0, CleanupIntervalS: 0})

	got := ops.Snapshot()
	assert.Equal(t, 100, got.MaxAttempts, "0 必须被忽略 —— 保留原值")
	assert.Equal(t, 7, got.SentRetentionDays, "0 必须被忽略 —— 保留原值")
}

func TestOps_ConcurrentApplyAndRead(t *testing.T) {
	t.Parallel()

	ops := NewOps(OpsConfig{MaxAttempts: 100, SentRetentionDays: 7, DeadRetentionDays: 30, CleanupIntervalS: 3600})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 写方：模拟 Nacos 反复推送
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 200; i++ {
			ops.Apply(OpsConfig{MaxAttempts: i, SentRetentionDays: i, DeadRetentionDays: i, CleanupIntervalS: i})
		}
		close(stop)
	}()

	// 读方：模拟 relay / 清理协程持续读
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = ops.Snapshot()
				}
			}
		}()
	}

	wg.Wait()
	require.NotNil(t, ops)
}

// TestRelay_UsesOpsSnapshot relay 的死信判定必须读 Ops 的当前值，
// 而不是构造时拷贝的那份 —— 否则热更对重试阈值完全无效。
func TestRelay_UsesOpsSnapshot(t *testing.T) {
	t.Parallel()

	ops := NewOps(OpsConfig{MaxAttempts: 5})
	r := NewRelay(repository.NewInMemoryOutboxRepo(), nil, 0, 1)
	r.Ops = ops

	// 把阈值调小
	ops.Apply(OpsConfig{MaxAttempts: 2})

	// relay 判定用的必须是 2 而不是构造时的 5
	assert.Equal(t, 2, r.currentMaxAttempts(),
		"relay 必须读 Ops 的当前快照，否则热更对死信阈值无效")
}
