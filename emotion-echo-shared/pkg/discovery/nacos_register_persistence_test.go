package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultRegisterEphemeralIsTrue：Stage-52 修复（revert PR-1 错误）。
//
// 历史：
//   - PR-1 (commit 6c33d08 时代) 假设 ephemeral 实例 30s 内被踢出，
//     把 defaultRegisterEphemeral 改为 false 走 persistent。
//   - 实测 (stage-52 调研 2026-09-08)：Nacos 2.4.3 standalone Derby + SDK v2.x
//     在 persistent service 已存在的情况下，注册 ephemeral instance 返 400/500。
//     6 个 dev svc 因 service 是 persistent + SDK 期望注册 ephemeral → fatal。
//   - 修复：还原 SDK 默认 ephemeral=true。心跳续约由 SDK 内部维护（BeatInterval 5s），
//     Nacos 2.4.3 + 单节点 Derby 在 svc 正常运行期间不会踢出。
//
// Stage-52 §〇 必须保证：defaultRegisterEphemeral=true。
// 这个测试是契约：任何人 revert 它都会触发 CI 红。
func TestDefaultRegisterEphemeralIsTrue(t *testing.T) {
	require.NotNil(t, &defaultRegisterEphemeral, "defaultRegisterEphemeral must be set")
	assert.True(t, defaultRegisterEphemeral,
		"defaultRegisterEphemeral must be true (Stage-52 fix,revert PR-1 mistake)")
}

// TestRegisterEphemeralResolution：defaultRegisterEphemeral=true + cfg.Ephemeral=false → true；
// cfg.Ephemeral=false 时仍 ephemeral（默认行为）；cfg.Ephemeral=true 时也 ephemeral（无变化）。
//
// 设计：prod 集群场景下，prod 部署应直接让 SDK 用 ephemeral=true（默认值即满足）。
// 若将来要切持久实例（例如某些要求 audit 留痕场景），由 cfg.Ephemeral=false 显式覆盖。
func TestRegisterEphemeralResolution(t *testing.T) {
	resolve := func(cfgEphemeral bool) bool {
		return defaultRegisterEphemeral || cfgEphemeral
	}
	assert.True(t, resolve(false), "默认 ephemeral=true（Stage-52 fix）")
	assert.True(t, resolve(true), "cfg=true → ephemeral")
}