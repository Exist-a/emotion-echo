// Package main — CL-F-16 契约钉：Postgres 启动失败的**重试 + fail-fast**装配
//
// 背景（CL-F-16，2026-10-16 Lane-CL-P 控制实验实测）：
//   user-svc 的 Postgres 连接是**裸的单次调用**（不重试）；失败即带着 nil repo
//   降级启动，且**此后永不自愈** —— 实验：停 postgres → 重启 user-svc（降级启动）
//   → 起 postgres（~12s 后 healthy）→ 观察 **300s，/health/ready 恒 503**。
//   证据：docs/closure-roadmap/stages/cl-09-backlog-paydown/evidence/clf16-readiness-selfheal-ab.txt
//
// 本钉锁住修复的**两条装配线**（缺任一条，测试 FAIL）：
//   ① 连接走共享 `dbconnect.ConnectWithRetry`（盖过"依赖稍后就绪"的启动竞态，
//      与 ai/analytics/assessment 三个服务同范式）；
//   ② 重试耗尽后按 `ShouldFailFast() && IsRequired("postgres")` 决定**退出启动**
//      —— 容器退出后由 compose 的 `restart: on-failure` 重试，依赖就绪时自然恢复
//      （这才是"自愈"的正解；dev 默认非 strict，保持"降级启动 + 如实报 degraded"的既有策略）。
//
// 为什么用源码级契约钉：这是**装配**问题（"分支有没有接上"），不是算法问题；
// 与仓内既有体例一致（web-bff/main_nacos_failfast_wiring_test.go）。
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func readMainSrc(t *testing.T) string {
	t.Helper()
	bs, err := os.ReadFile("main.go")
	assert.NoError(t, err, "读不到 user-svc main.go")
	return string(bs)
}

func TestMain_PostgresUsesConnectWithRetry(t *testing.T) {
	body := readMainSrc(t)

	assert.Contains(t, body, "dbconnect.ConnectWithRetry",
		"Postgres 连接必须走共享 dbconnect.ConnectWithRetry（CL-F-16：单次连接挡不住启动竞态）")
}

func TestMain_PostgresFailFastWiring(t *testing.T) {
	body := readMainSrc(t)

	assert.Contains(t, body, "ShouldFailFast()",
		"启动期依赖失败必须接 ShouldFailFast()（STARTUP_STRICT）")
	assert.Contains(t, body, `IsRequired("postgres")`,
		"postgres 必须被显式标为可 fail-fast 的依赖（STARTUP_STRICT_DEPS）")
	assert.Contains(t, body, "refusing to start",
		"strict 模式必须日志明示 fail-fast（便于排查）")
	// dev 兼容：非 strict 时必须是"打日志继续"，不得无条件退出
	assert.Contains(t, body, "[postgres] connect failed after",
		"非 strict 模式必须保留降级启动（打日志继续），否则 dev 起栈会被 DB 抖动打断")
}

// IsRequired 的语义边界：postgres 必须在默认必需清单里（否则 strict 模式也不会退出）。
func TestStartupStrictDeps_IncludesPostgres(t *testing.T) {
	// 默认清单（未设 STARTUP_STRICT_DEPS 时）由 shared bootstrap 决定
	bs, err := os.ReadFile("../../emotion-echo-shared/pkg/bootstrap/deps.go")
	if err != nil {
		t.Skipf("shared bootstrap 源码不可达（构建环境差异）：%v", err)
	}
	src := string(bs)
	i := strings.Index(src, `list = "postgres`)
	assert.GreaterOrEqual(t, i, 0,
		"shared bootstrap 的默认 STARTUP_STRICT_DEPS 必须含 postgres（CL-F-16 自愈依赖它）")
}
