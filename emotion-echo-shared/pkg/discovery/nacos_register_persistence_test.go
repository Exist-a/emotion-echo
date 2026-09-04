package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultRegisterEphemeralIsFalse：PR-1 修复目标——默认注册持久实例。
// 这是 dev 模式注册表可信度的根因修复。
func TestDefaultRegisterEphemeralIsFalse(t *testing.T) {
	require.NotNil(t, &defaultRegisterEphemeral, "defaultRegisterEphemeral must be set")
	assert.False(t, defaultRegisterEphemeral, "defaultRegisterEphemeral must be false for dev reliability")
}

// TestRegisterEphemeralResolution：defaultRegisterEphemeral=false + cfg.Ephemeral=false → false；
// 任意一边为 true 即取 true（prod 集群显式覆盖）。
func TestRegisterEphemeralResolution(t *testing.T) {
	// 模拟 Register 的 ephemeral 解析逻辑（与 nacos_register.go 一致）
	resolve := func(cfgEphemeral bool) bool {
		return defaultRegisterEphemeral || cfgEphemeral
	}
	assert.False(t, resolve(false), "默认 + cfg=false → 持久")
	assert.True(t, resolve(true), "cfg=true → ephemeral（prod 覆盖路径）")
}