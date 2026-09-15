// Package main — Round 4.2 P1-9 Nacos fail-fast caller-wiring test
//
// 锁住 main.go 在 BootNacos 失败时若 STARTUP_STRICT_DEPS=true → os.Exit(1)，
// 删除该分支 → 测试 FAIL，CI 立刻挂。
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMain_NacosFailFast_StrictModeExits1 STARTUP_STRICT_DEPS=true 时 Nacos 失败必须 os.Exit(1)
func TestMain_NacosFailFast_StrictModeExits1(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(src)

	// 必须有 ShouldFailFast() 分支
	assert.Contains(t, body, "ShouldFailFast()",
		"web-bff 必须读 STARTUP_STRICT_DEPS（Round 4.2 fail-fast）")
	// 必须有 os.Exit(1)
	assert.True(t, strings.Contains(body, "os.Exit(1)"),
		"web-bff Nacos 失败 strict 模式必须 os.Exit(1)")
	// strict 模式 log 必须明确"refusing to start"
	assert.Contains(t, body, "refusing to start",
		"必须日志明示 strict 模式 fail-fast（便于排查）")
	// dev 模式保留 swallow 兼容
	assert.Contains(t, body, "continuing",
		"dev 模式必须保留 swallow (continuing) 行为（向后兼容）")
}
