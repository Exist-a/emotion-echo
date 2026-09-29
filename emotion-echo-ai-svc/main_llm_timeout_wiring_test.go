// Package main — E2E-23 P2：`LLM.Timeout` 死配置的接线修复测试
//
// 🔴 缺陷（E2E-F-159，plan 期实测确认）：
//
//	main.go 的 applyEnvOverrides 确实把 env `LLM_TIMEOUT` 读进了 `c.LLM.Timeout`
//	（main.go:152），yaml 也有 `Timeout: 3`（etc/ai-api.yaml:59），
//	config.SetDefaults 也给了默认 3（internal/config/config.go:152-154），
//	**但 main.go:487 构造 `fusion.NewLLMFuser` 时只传了 BaseURL/APIKey/Model，
//	根本没传 Timeout** ⇒ 该字段永远是 0 ⇒ `NewLLMFuser` 永远走内置的 3s 兜底。
//	结果：yaml 里配的 3 与 env 里的 5 全部无效，**"配了但不生效"**。
//
// 本测试是**行为断言**而非源码字符串匹配：直接验证"配置值是否真的到达
// LLMFuser 内部的 http.Client"。仓内既有的 main_*_test.go 多为源码字符串
// 断言（那是它锁 env 读取顺序的合理手段），但本条要锁的是**值有没有传下去**
// —— 字符串匹配只能证明"代码里出现过这个字段名"，证明不了"它被用上了"。
package main

import (
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/fusion"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLLMFuserForConfig_TimeoutReachesHTTPClient 核心断言：
// 配置的秒数必须真的变成 http.Client 的 Timeout。
func TestNewLLMFuserForConfig_TimeoutReachesHTTPClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		timeoutSecs int
		wantTimeout time.Duration
	}{
		{
			name:        "configured 5s reaches client",
			timeoutSecs: 5,
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "configured 30s reaches client",
			timeoutSecs: 30,
			wantTimeout: 30 * time.Second,
		},
		{
			// 0 时保持 NewLLMFuser 的内置兜底（Stage 35 PR-4 的 3s 决策不能被改掉）
			name:        "zero falls back to builtin 3s",
			timeoutSecs: 0,
			wantTimeout: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newLLMFuserForConfig("https://example.invalid", "k", "deepseek-chat", tt.timeoutSecs)
			require.NotNil(t, f, "接线函数必须返回非 nil fuser")

			got := f.HTTPTimeout()
			assert.Equal(t, tt.wantTimeout, got,
				"配置的超时必须真正到达 http.Client —— 否则 yaml/env 里配的值全是摆设")
		})
	}
}

// TestNewLLMFuserForConfig_UnitIsSeconds 锁住单位换算：
// config.LLM.Timeout 是 int（秒），LLMConfig.Timeout 是 time.Duration。
// 少乘 time.Second 会得到 5**纳秒**，比不传还糟（立刻超时）。
func TestNewLLMFuserForConfig_UnitIsSeconds(t *testing.T) {
	t.Parallel()

	f := newLLMFuserForConfig("https://example.invalid", "k", "m", 7)
	got := f.HTTPTimeout()

	assert.Equal(t, 7*time.Second, got)
	assert.Greater(t, got, time.Microsecond,
		"若结果在微秒/纳秒量级，说明 int 秒没乘 time.Second —— 会让每次 LLM 调用立刻超时")
}

// TestLLMFuser_HTTPTimeout_DefaultFallback 兜底语义不得被接线改动破坏。
func TestLLMFuser_HTTPTimeout_DefaultFallback(t *testing.T) {
	t.Parallel()

	// 直接构造 fusion.LLMFuser（不经接线函数）——零值 Timeout 仍应是 3s
	f := fusion.NewLLMFuser(fusion.LLMConfig{BaseURL: "https://example.invalid", APIKey: "k", Model: "m"})
	assert.Equal(t, 3*time.Second, f.HTTPTimeout(),
		"NewLLMFuser 的内置 3s 兜底是 Stage 35 PR-4 的刻意决策（5s tick 下 10s 会卡死 tick），不得被改")
}
