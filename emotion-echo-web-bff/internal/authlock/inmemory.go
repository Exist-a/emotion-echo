// Round 4.3 后补：LoginLockStore 的 in-memory 实现。
//
// 行为与 auth_handler.go 原 in-memory map 完全一致（保证向后兼容）。
// 主要为 BFF 单实例部署或 Redis 不可达时的 fallback 使用。
package authlock

import (
	"context"
	"sync"
	"time"
)

// InMemoryStore Round 4.3 后补：进程内 LoginLockStore 实现。
//
// 字段与原 auth_handler.go:60-94 完全一致（保留原行为）。
// 线程安全：用 sync.RWMutex 保护 map。
type InMemoryStore struct {
	mu          sync.RWMutex
	failures    map[string]*loginAttempt    // username → 失败计数
	verCodes    map[string]*verificationCode // username → 验证码 + 元数据
	lockWindow  time.Duration              // 锁定窗口（默认 15min）
	maxFailures int                        // 最大失败次数（默认 5）
	minGap      time.Duration              // 验证码最小间隔（默认 60s）
}

// loginAttempt 跟踪某 username 的登录失败次数与锁定状态
type loginAttempt struct {
	failCount int
	lockedAt  time.Time
}

// verificationCode 验证码缓存（包含 code + 过期 + 上次发送时间）
type verificationCode struct {
	code       string
	expiresAt  time.Time
	lastSentAt time.Time
}

// NewInMemoryStore 构造 InMemoryStore。
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		failures:    make(map[string]*loginAttempt),
		verCodes:    make(map[string]*verificationCode),
		lockWindow:  loginLockWindow,
		maxFailures: loginMaxFailures,
		minGap:      verificationMinGap,
	}
}

// IsLocked Round 4.3 后补：检查 username 是否被锁（在 lockWindow 锁定期内）
func (s *InMemoryStore) IsLocked(_ context.Context, username string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attempt, ok := s.failures[username]
	if !ok {
		return false
	}
	if !attempt.lockedAt.IsZero() && time.Since(attempt.lockedAt) < s.lockWindow {
		return true
	}
	// 锁定窗口已过 → 重置（行为与原 auth_handler.go 一致）
	return false
}

// RecordFailure Round 4.3 后补：记录一次登录失败。返 true 表示这次失败触发了锁定。
//
// 行为与原 recordFailure 函数一致：
//   - 已锁定用户 → 不再累加计数（防锁定期内 failCount 叠加）
//   - failCount ≥ maxFailures → 触发锁定 + 重置计数
func (s *InMemoryStore) RecordFailure(_ context.Context, username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.failures[username]
	if !ok {
		attempt = &loginAttempt{}
		s.failures[username] = attempt
	}
	// 已锁定用户 → 不再累加计数
	if !attempt.lockedAt.IsZero() && time.Since(attempt.lockedAt) < s.lockWindow {
		return false
	}
	attempt.failCount++
	if attempt.failCount >= s.maxFailures {
		attempt.lockedAt = time.Now()
		attempt.failCount = 0 // 重置计数，锁定期内不再叠加
		return true
	}
	return false
}

// ClearFailures Round 4.3 后补：清除 username 的失败计数（登录成功时调用）
func (s *InMemoryStore) ClearFailures(_ context.Context, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, username)
	return nil
}

// CanSendVerificationCode Round 4.3 后补：检查 verificationMinGap 间隔
func (s *InMemoryStore) CanSendVerificationCode(_ context.Context, username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.verCodes[username]
	if !ok {
		return true
	}
	return time.Since(entry.lastSentAt) >= s.minGap
}

// SaveVerificationCode Round 4.3 后补：保存 username 对应的验证码
func (s *InMemoryStore) SaveVerificationCode(_ context.Context, username, code string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.verCodes[username] = &verificationCode{
		code:       code,
		expiresAt:  now.Add(ttl),
		lastSentAt: now,
	}
	return nil
}

// GetVerificationCode Round 4.3 后补：取验证码（不存在或过期返 ("", nil)）
func (s *InMemoryStore) GetVerificationCode(_ context.Context, username string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.verCodes[username]
	if !ok {
		return "", nil
	}
	if time.Now().After(entry.expiresAt) {
		return "", nil
	}
	return entry.code, nil
}

// 编译期断言：InMemoryStore 同时实现两个接口
// （登录锁定可走 Redis 或 in-memory；验证码只允许 in-memory，见 store.go 裁定说明）
var _ LoginLockStore = (*InMemoryStore)(nil)
var _ VerificationCodeStore = (*InMemoryStore)(nil)