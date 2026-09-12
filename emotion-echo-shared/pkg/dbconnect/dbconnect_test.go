// Package dbconnect — 启动期 Postgres 连接重试的单元测试
//
// Stage 77 RED：dev 栈实测（stage-76 §二.3）瞬时 DNS 故障下 openPostgres 一次失败
// → svc 带 nil repo 启动 → 之后所有触 repo 的 RPC panic。本包锁定"有限次退避重试"契约：
//   - 首次成功不 sleep
//   - 失败后按 backoff sleep 再试，成功即返回
//   - 次数耗尽返回最后一次错误（sleep 不在最后一次失败后发生）
package dbconnect

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectWithRetry_FirstTrySucceeds(t *testing.T) {
	calls := 0
	var sleeps []time.Duration

	got, err := ConnectWithRetry(func() (string, error) {
		calls++
		return "ok", nil
	}, 3, 100*time.Millisecond, func(d time.Duration) { sleeps = append(sleeps, d) })

	require.NoError(t, err)
	assert.Equal(t, "ok", got)
	assert.Equal(t, 1, calls, "首次成功不应重试")
	assert.Empty(t, sleeps, "首次成功不应 sleep")
}

func TestConnectWithRetry_RetriesThenSucceeds(t *testing.T) {
	calls := 0
	var sleeps []time.Duration

	got, err := ConnectWithRetry(func() (string, error) {
		calls++
		if calls <= 2 {
			return "", errors.New("dns transient failure")
		}
		return "ok", nil
	}, 5, 500*time.Millisecond, func(d time.Duration) { sleeps = append(sleeps, d) })

	require.NoError(t, err)
	assert.Equal(t, "ok", got)
	assert.Equal(t, 3, calls)
	assert.Equal(t, []time.Duration{500 * time.Millisecond, 500 * time.Millisecond}, sleeps,
		"每次失败后应 sleep 一个 backoff")
}

func TestConnectWithRetry_ExhaustedReturnsLastError(t *testing.T) {
	calls := 0
	var sleeps []time.Duration
	lastErr := errors.New("still down after 3 attempts")

	_, err := ConnectWithRetry(func() (string, error) {
		calls++
		return "", lastErr
	}, 3, 50*time.Millisecond, func(d time.Duration) { sleeps = append(sleeps, d) })

	require.Error(t, err)
	assert.ErrorIs(t, err, lastErr, "耗尽后应返回最后一次错误")
	assert.Equal(t, 3, calls, "恰好尝试 attempts 次")
	assert.Len(t, sleeps, 2, "最后一次失败后不应再 sleep")
}

func TestConnectWithRetry_AttemptsBelowOne_NormalizedToOne(t *testing.T) {
	calls := 0
	_, err := ConnectWithRetry(func() (string, error) {
		calls++
		return "", errors.New("down")
	}, 0, time.Millisecond, func(time.Duration) { t.Fatal("attempts<1 不应 sleep") })

	require.Error(t, err)
	assert.Equal(t, 1, calls, "attempts<1 应归一化为至少尝试 1 次")
}
