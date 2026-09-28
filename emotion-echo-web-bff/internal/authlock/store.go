// Package authlock 提供 AuthHandler 用的登录失败计数 / 验证码缓存存储。
//
// E2E-20 多实例并发修复（plan §2.B / 测试点 #6+#7）：
//   - 原实现：in-memory map 进程内（auth_handler.go:60-94），多实例下静默失效
//   - 新接口：LoginLockStore（in-memory + redis 双实现，env 决定）
//   - 多 BFF 实例时用 redis 实现，跨实例计数一致
//
// 调用方：auth_handler.go 通过 Store 接口访问；main.go 根据
// LOGIN_LOCK_BACKEND=inmemory|redis env 决定 New 新哪种实现。
package authlock

import (
	"context"
	"errors"
	"time"
)

// LoginLockStore Round 4.3 后补：登录失败计数 + 验证码缓存存储抽象。
//
// 设计要点：
//   - 接口一致：in-memory 与 redis 实现必须行为一致（同语义）
//   - 降级：Redis 不可达时 in-memory fallback（不更糟，详见 plan §6 [M]）
//   - 时间语义：所有"时间窗口"判断由 backend 自管理（不依赖 ctx deadline）
type LoginLockStore interface {
	// IsLocked 检查 username 是否被锁（在 loginLockWindow 锁定期内）
	IsLocked(ctx context.Context, username string) bool

	// RecordFailure 记录一次登录失败。返 true 表示这次失败触发了锁定。
	// 锁定期内重复失败不增加计数（避免恶意打爆 failCount）。
	RecordFailure(ctx context.Context, username string) bool

	// ClearFailures 清除 username 的失败计数（登录成功时调用）
	ClearFailures(ctx context.Context, username string) error

	// CanSendVerificationCode 检查 username 是否在 verificationMinGap
	// 间隔内已发过验证码（防止枚举攻击）
	CanSendVerificationCode(ctx context.Context, username string) bool

	// SaveVerificationCode 保存 username 对应的验证码 + TTL
	// ttl 应 = verificationCodeTTL（如 5 分钟）
	SaveVerificationCode(ctx context.Context, username, code string, ttl time.Duration) error

	// GetVerificationCode 取 username 对应的验证码（登录/注册时验证）
	// 不存在或过期返 ("", nil)
	GetVerificationCode(ctx context.Context, username string) (string, error)
}

// 常量：与 auth_handler.go 既有常量保持一致（行为兼容）
const (
	loginMaxFailures   = 5
	loginLockWindow    = 15 * time.Minute
	verificationMinGap = 60 * time.Second
	verificationTTL    = 5 * time.Minute
)

// ErrStoreDown Round 4.3 后补：后端不可达（Redis down 等）
// 调用方可选降级到 in-memory；非降级场景（如 redis 不可达但 in-memory backend）
// 则直接返此 error 触发 5xx 响应。
var ErrStoreDown = errors.New("authlock store: backend down")

// implementationRound Round 4.3 后补：实现版本标识（用于 audit / 报告）
const implementationRound = "Round 4.3 后补 + E2E-20 #6+#7"