// Package main — E2E-18 测试点 #3：LRU capacity env 默认值契约（RED 先行）
//
// 背景（E2E-18 plan §2.A，2026-09-24 计划期实测）：
//   - main.go 原实现 readEnvInt("WORKER_LRU_CAPACITY", 0) ⇒ 未设 env 时
//     cap=0 ⇒ if cap > 0 不成立 ⇒ rateLimit=nil，**LRU 在所有部署里从不构造**；
//   - 与同文件注释「默认 cap=1024 / TTL=4min」、架构决策 15「LRU(cap=1024) 已生效」
//     三对一矛盾（deploy/ 全树零 WORKER_LRU env，运行日志无 enabled 行实证）。
//
// 契约：
//  1. 未设 WORKER_LRU_CAPACITY → 默认启用 cap=1024（对齐注释与决策 15）
//  2. 显式设置正值 → 覆盖生效
//  3. 显式 =0 → 关闭（保留逃生门；readEnvInt 对显式 "0" 返回 0 而非 fallback）
package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLRUCapacityFromEnv_DefaultEnabled1024(t *testing.T) {
	os.Unsetenv("WORKER_LRU_CAPACITY")
	assert.Equal(t, 1024, lruCapacityFromEnv(),
		"未设 WORKER_LRU_CAPACITY 时应默认启用 cap=1024（决策 15）")
}

func TestLRUCapacityFromEnv_ExplicitOverride(t *testing.T) {
	os.Setenv("WORKER_LRU_CAPACITY", "512")
	defer os.Unsetenv("WORKER_LRU_CAPACITY")
	assert.Equal(t, 512, lruCapacityFromEnv(),
		"显式正值应覆盖默认")
}

func TestLRUCapacityFromEnv_ExplicitZeroDisables(t *testing.T) {
	os.Setenv("WORKER_LRU_CAPACITY", "0")
	defer os.Unsetenv("WORKER_LRU_CAPACITY")
	assert.Equal(t, 0, lruCapacityFromEnv(),
		"显式 =0 应关闭 LRU（逃生门；0 不是非法值不得回落默认）")
}
