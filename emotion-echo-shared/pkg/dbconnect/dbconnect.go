// Package dbconnect — 启动期 Postgres 连接的有限次退避重试。
//
// Stage 77（stage-76 §二.3 的修复）：dev 栈实测瞬时 DNS 故障下 openPostgres
// 一次失败 → svc 按"dev 不阻断"策略带 nil repo 启动 → 之后所有触 repo 的
// RPC panic。本包在"不阻断启动"与"裸 nil repo"之间加一层盖板：
// 连接失败时按 backoff 有限次重试，盖过秒级基础设施抖动；重试耗尽才降级。
package dbconnect

import "time"

// 默认重试参数（各 svc main.go 直接引用）：
// 10 次 × 500ms ≈ 5s 覆盖窗口——足以盖过 Docker 网络重建/瞬时 DNS 抖动，
// 又不会把"数据库真挂了"的启动拖延到不可接受。
const (
	DefaultAttempts = 10
	DefaultBackoff  = 500 * time.Millisecond
)

// ConnectWithRetry 以固定 backoff 有限次重试 connect，成功即返回。
// sleep 由调用方注入（生产传 time.Sleep，测试注入收集器），每次失败后
// sleep 一次（最后一次失败除外）。attempts < 1 归一化为 1（至少尝试一次）。
func ConnectWithRetry[T any](connect func() (T, error), attempts int, backoff time.Duration, sleep func(time.Duration)) (T, error) {
	if attempts < 1 {
		attempts = 1
	}
	var zero T
	var lastErr error
	for i := 0; i < attempts; i++ {
		result, err := connect()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if i < attempts-1 {
			sleep(backoff)
		}
	}
	return zero, lastErr
}
