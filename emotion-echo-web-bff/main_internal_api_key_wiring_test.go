// Package main — Round 3.5 per-svc API key isolation caller-wiring test
//
// 锁住 main.go 读取 BFF_LLM_INTERNAL_API_KEY env（per-svc 隔离），
// 删除该行 → 测试 FAIL，CI 立刻挂。
//
// 背景：
//   - ai-svc / web-bff / llm-service 共享 INTERNAL_API_KEY 一个 env 名（违反最小权限）
//   - Round 3.5 修复：web-bff 优先读 BFF_LLM_INTERNAL_API_KEY → fallback INTERNAL_API_KEY
//   - 同样模式：ai-svc 优先 AI_LLM_INTERNAL_API_KEY
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMain_BFFLLMInternalAPIKey_EnvWiring main.go 必须读取 BFF_LLM_INTERNAL_API_KEY（per-svc 隔离）
func TestMain_BFFLLMInternalAPIKey_EnvWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	assert.Contains(t, string(src), "BFF_LLM_INTERNAL_API_KEY",
		"web-bff main.go 必须读取 BFF_LLM_INTERNAL_API_KEY env（Round 3.5 跨 svc 隔离）")
	// 同时确认 fallback 到共享 INTERNAL_API_KEY（向后兼容）
	assert.Contains(t, string(src), "INTERNAL_API_KEY",
		"web-bff 必须保留 INTERNAL_API_KEY 作为 fallback（向后兼容）")
}

// TestMain_EnvOrFallsBackToLegacyInternalAPIKey 锁死 envOr 调用形式
// —— 第二个参数必须是 os.Getenv("INTERNAL_API_KEY") 而不是硬编码字符串。
func TestMain_EnvOrFallsBackToLegacyInternalAPIKey(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(src)
	// 必须有 `envOr("BFF_LLM_INTERNAL_API_KEY", os.Getenv("INTERNAL_API_KEY"))` 模式
	assert.True(t, strings.Contains(body, `envOr("BFF_LLM_INTERNAL_API_KEY", os.Getenv("INTERNAL_API_KEY"))`),
		"web-bff main.go 必须用 envOr(\"BFF_LLM_INTERNAL_API_KEY\", os.Getenv(\"INTERNAL_API_KEY\")) 形式")
}
