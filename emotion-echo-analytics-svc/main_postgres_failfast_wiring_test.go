// Package main — CL-F-16 契约钉：analytics-svc 的 Postgres fail-fast 装配
//
// 与 user-svc / chat-svc 同批（CL-F-16）：重试已在，缺的是"重试耗尽后按
// STARTUP_STRICT 退出启动" —— 否则 strict/prod 下容器既不健康也永不退出。
package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain_PostgresFailFastWiring(t *testing.T) {
	bs, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(bs)

	assert.Contains(t, body, "dbconnect.ConnectWithRetry",
		"analytics-svc 的 Postgres 连接必须走 dbconnect.ConnectWithRetry")
	assert.Contains(t, body, "ShouldFailFast()",
		"启动期依赖失败必须接 ShouldFailFast()（STARTUP_STRICT）")
	assert.Contains(t, body, `IsRequired("postgres")`,
		"postgres 必须被显式标为可 fail-fast 的依赖")
	assert.Contains(t, body, "refusing to start",
		"strict 模式必须日志明示 fail-fast（便于排查）")
	assert.Contains(t, body, "[postgres] connect failed after",
		"非 strict 必须保留降级启动")
}
