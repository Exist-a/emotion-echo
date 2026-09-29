// Package outbox — 运营参数的并发安全容器（E2E-23 E 组 / D-32）
//
// 背景：E2E-23 计划期盘点出 chat-svc 有 4 个真正值得热调的运营参数
// （Outbox.MaxAttempts / SentRetentionDays / DeadRetentionDays /
// CleanupIntervalS），但当前实现有两个问题：
//
//  1. **热更无载体**：nacos_boot.go 拿到 `{svc}.ops.yaml` 后只打一行
//     `ops config loaded: %d bytes` 就丢弃（F-155），配置层形同虚设。
//
//  2. **直接改 config 会引入 data race**：main.go 的清理协程在 ticker
//     闭包里读 `c.Outbox.SentRetentionDays`（main.go:248），而 `c` 是
//     main() 的**局部变量**。若 Nacos 回调直接写 `c.Outbox.*`，
//     就是一个 goroutine 写、另一个 goroutine 读的**真实数据竞争**
//     （E2E-21 教训：单测全绿也照样是 bug，只有 -race 抓得到）。
//
// 故引入本容器：Nacos 回调只写它，relay / 清理协程只读它，
// 中间用 sync/atomic 隔离。
//
// 范式参照 web-bff 的 internal/handler/hotreload.go（HotReloadLimiter），
// 但那边只有"一个中间件读"、这边是"清理协程按 tick 读"且**并发写**，
// 故用 atomic 而非 RWMutex —— 后者需要每个读点都记得配 RLock，
// 漏一处就退化回 race。
package outbox

import "sync/atomic"

// OpsConfig 是 ops dataId（`{svc}.ops.yaml`）的**白名单**字段集。
//
// 刻意用固定结构体而非 map[string]any：未知 key 会被 yaml 静默忽略，
// 这本身就是一层防御（P1 敏感字段保护见 shared/pkg/configcenter/ops_sanitize.go）。
//
// 单位说明：MaxAttempts 是"次"，其余三个是"天/秒"，与 config.Outbox 同名
// 字段保持一致，避免两套单位。
type OpsConfig struct {
	// MaxAttempts outbox 重试上限，超出则标记 dead（0 = 关闭 dead 状态机）
	MaxAttempts int `yaml:"max_attempts"`
	// SentRetentionDays 已发送记录保留天数
	SentRetentionDays int `yaml:"sent_retention_days"`
	// DeadRetentionDays 死信保留天数（排障证据，勿设过短）
	DeadRetentionDays int `yaml:"dead_retention_days"`
	// CleanupIntervalS 清理任务周期（秒）
	CleanupIntervalS int `yaml:"cleanup_interval_s"`
}

// OpsSnapshot 是读取侧的不可变快照。
type OpsSnapshot struct {
	MaxAttempts       int
	SentRetentionDays int
	DeadRetentionDays int
	CleanupIntervalS  int
}

// Ops 是运营参数的并发安全容器。
type Ops struct {
	maxAttempts       atomic.Int64
	sentRetentionDays atomic.Int64
	deadRetentionDays atomic.Int64
	cleanupIntervalS  atomic.Int64
}

// NewOps 用初始配置构造（来自 config.Outbox，启动时读 yaml/env）。
func NewOps(init OpsConfig) *Ops {
	o := &Ops{}
	o.setAll(init)
	return o
}

func (o *Ops) setAll(c OpsConfig) {
	// 0 视为"未设置"：保留既有默认，不让空 dataId 把阈值清零。
	// MaxAttempts=0 会关闭 dead 状态机；SentRetentionDays=0 会立即清库 ——
	// 两种都不是"安全的默认值"，因此宁可保持不变。
	if c.MaxAttempts > 0 {
		o.maxAttempts.Store(int64(c.MaxAttempts))
	}
	if c.SentRetentionDays > 0 {
		o.sentRetentionDays.Store(int64(c.SentRetentionDays))
	}
	if c.DeadRetentionDays > 0 {
		o.deadRetentionDays.Store(int64(c.DeadRetentionDays))
	}
	if c.CleanupIntervalS > 0 {
		o.cleanupIntervalS.Store(int64(c.CleanupIntervalS))
	}
}

// Apply 应用来自 Nacos 的新值（Nacos ListenConfig 回调调用）。
// 零值字段被忽略 —— 理由同 setAll。
func (o *Ops) Apply(c OpsConfig) { o.setAll(c) }

// Snapshot 返回当前值的副本（relay / 清理协程调用）。
func (o *Ops) Snapshot() OpsSnapshot {
	if o == nil {
		return OpsSnapshot{}
	}
	return OpsSnapshot{
		MaxAttempts:       int(o.maxAttempts.Load()),
		SentRetentionDays: int(o.sentRetentionDays.Load()),
		DeadRetentionDays: int(o.deadRetentionDays.Load()),
		CleanupIntervalS:  int(o.cleanupIntervalS.Load()),
	}
}
