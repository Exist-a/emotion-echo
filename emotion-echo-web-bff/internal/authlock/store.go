// Package authlock 提供 AuthHandler 用的登录失败计数 / 验证码缓存存储。
//
// E2E-20 多实例并发修复（plan §2.B / 测试点 #6+#7）：
//   - 原实现：in-memory map 进程内（auth_handler.go:60-94），多实例下静默失效
//   - 新接口：LoginLockStore（in-memory + redis 双实现，env 决定）
//   - 多 BFF 实例时用 redis 实现，跨实例计数一致
//
// E2E-20 收尾裁定（D-01 + 用户 2026-09-28 确认）：验证码存储从本包的
// Redis 化范围中移出，独立为 VerificationCodeStore（仅 in-memory 实现）。
// 原因：/api/v1/auth/verification-code 端点是 D-01 决议下的遗留物
// （注册验证码步骤已删、找回密码改密保问题），待删除（E2E-F-144），
// 不应为其做跨实例共享存储。
//
// 调用方：auth_handler.go 通过两个 Store 接口访问；main.go 根据
// LOGIN_LOCK_BACKEND=inmemory|redis env 决定登录锁定用哪种实现。
package authlock

import (
	"context"
	"errors"
	"time"
)

// LoginLockStore Round 4.3 后补：登录失败计数存储抽象。
//
// 设计要点：
//   - 接口一致：in-memory 与 redis 实现必须行为一致（同语义）
//   - 降级：Redis 不可达时不 fail-closed（不更糟，详见 plan §6 [M]）
//   - 时间语义：所有"时间窗口"判断由 backend 自管理（不依赖 ctx deadline）
type LoginLockStore interface {
	// IsLocked 检查 username 是否被锁（在 loginLockWindow 锁定期内）
	IsLocked(ctx context.Context, username string) bool

	// RetryAfter 返回 username 的**剩余**锁定时间；未锁定返 0。
	//
	// E2E-29 遗留项 1（2026-10-08）：供 handler 在 423 响应写 `Retry-After`
	// 头（RFC 7231 §7.1.3），让前端精确退避而非恒等满窗口。
	RetryAfter(ctx context.Context, username string) time.Duration

	// RecordFailure 记录一次登录失败。返 true 表示这次失败触发了锁定。
	// 锁定期内重复失败不增加计数（避免恶意打爆 failCount）。
	RecordFailure(ctx context.Context, username string) bool

	// ClearFailures 清除 username 的失败计数（登录成功时调用）
	ClearFailures(ctx context.Context, username string) error
}

// VerificationCodeStore E2E-20 收尾拆出：验证码缓存存储。
//
// ⚠️ 裁定（D-01 + 用户 2026-09-28）：/api/v1/auth/verification-code 端点
// 为遗留物（注册验证码步骤已按 D-01 删除、找回密码改密保问题），待删除
// （账本 E2E-F-144）。因此本接口：
//   - 只允许 in-memory 实现（单实例语义即可，端点活不到多实例那天）
//   - 禁止再 Redis 化 / 跨实例共享（契约测试 TestInterfaceShrink 锁死）
type VerificationCodeStore interface {
	// CanSendVerificationCode 检查 username 是否在 verificationMinGap
	// 间隔内已发过验证码（防止枚举攻击）
	CanSendVerificationCode(ctx context.Context, username string) bool

	// SaveVerificationCode 保存 username 对应的验证码 + TTL
	// ttl 应 = verificationTTL
	SaveVerificationCode(ctx context.Context, username, code string, ttl time.Duration) error

	// GetVerificationCode 取 username 对应的验证码（注册时验证）
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