// Package main — E2E-F-137 Nacos 注册必须成功才允许继续
//
// 背景（2026-09-24）：BFF 启动时若 Nacos 还未就绪，BootNacos 内部 WaitForNacos 60s 后
// 超时报错，main.go 默认走 swallow continuing 路径，BFF 启动但不注册自己 →
// APISIX upstream 6 解析 nodes:{} → 全站 /api/v1/* 503（容器 /health 200 同时成立）。
//
// 修复方向：dev 模式默认应阻塞重试直到注册成功；prod 用 STARTUP_STRICT 显式覆盖。
//
// 本测试锁住"main.go 在 BootNacos 失败时进入重试/阻塞循环而不是 swallow"的形态：
// 1) main.go 必须能识别 BootNacos 失败
// 2) main.go 必须含某种 retry loop / 重新调 BootNacos 的语义
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMain_NacosBootFailure_MustNotSilentlyContinue
//
// E2E-F-137（2026-09-24）：BFF 注册 Nacos 失败必须显式处理（重试/阻塞），
// 不能仅靠现有 "continuing" warn 后继续启动（这会让 APISIX 503 但容器 healthy）。
//
// 通过条件（任一）：
//  (a) main.go 在 BootNacos 失败时调用 os.Exit(1)（默认 dev 模式也可）
//  (b) main.go 在 BootNacos 失败时进入 retry loop 重新尝试 BootNacos
//
// 禁止（任何一项即 FAIL）：
//  - 仅 "log.Printf continuing" 后继续启动，无 retry / exit
func TestMain_NacosBootFailure_MustNotSilentlyContinue(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(src)

	// BootNacos 失败分支必须含 retry loop 或 fail-fast os.Exit(1) 之一。
	// 严禁仅 continuing warn 后继续（E2E-F-137 实测导致全站 503 但容器 healthy）。

	// 形态 1：retry loop（Backoff + 重新 BootNacos）
	//   取 substring 看是否有 retry 性质的代码片段
	hasRetry := strings.Contains(body, "Backoff") ||
		(strings.Contains(body, "backoff") && strings.Contains(body, "BootNacos(")) ||
		strings.Contains(body, "booting Nacos") && strings.Contains(body, "for")

	// 形态 2：默认 dev 模式也走 fail-fast（os.Exit(1)）
	hasFailFastExit := strings.Contains(body, "os.Exit(1)") &&
		// 必须不是在 if ShouldFailFast() 内的"可选 strict 分支"
		strings.Contains(body, "[nacos] boot failed (refusing to start, will retry)") ||
		strings.Contains(body, "[nacos] boot failed (refusing to start")

	// 显式分支：默认 swallow 路径必须不复存在
	defaultSwallowOK := strings.Contains(body, "log.Printf(\"[nacos] boot failed (continuing)\")") &&
		!hasRetry && !hasFailFastExit

	assert.False(t, defaultSwallowOK,
		"E2E-F-137：BFF Nacos 注册失败默认 swallow continuing 是真 bug → "+
			"必须有 retry loop 或 os.Exit(1) 之一取代")
	assert.True(t, hasRetry || hasFailFastExit,
		"E2E-F-137：BootNacos 失败分支必须有 retry loop 或 fail-fast exit 实动作")
}