// Package main — Round 3.5 per-svc API key isolation caller-wiring test
//
// 锁住 main.go 读取 AI_LLM_INTERNAL_API_KEY env（per-svc 隔离），
// 删除该行 → 测试 FAIL，CI 立刻挂。
package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMain_AILLMInternalAPIKey_EnvWiring main.go 必须读取 AI_LLM_INTERNAL_API_KEY（per-svc 隔离）
func TestMain_AILLMInternalAPIKey_EnvWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(src)
	assert.Contains(t, body, "AI_LLM_INTERNAL_API_KEY",
		"ai-svc main.go 必须读取 AI_LLM_INTERNAL_API_KEY env（Round 3.5 跨 svc 隔离）")
	assert.Contains(t, body, "INTERNAL_API_KEY",
		"ai-svc 必须保留 INTERNAL_API_KEY 作为 fallback（向后兼容）")
	// 顺序：os.Getenv("AI_LLM_INTERNAL_API_KEY") 必须在 os.Getenv("INTERNAL_API_KEY") 之前
	// （注释里的字符串不计入，只看真实调用位置）
	aiCallIdx := stringsIndex(body, `os.Getenv("AI_LLM_INTERNAL_API_KEY")`)
	sharedCallIdx := stringsIndex(body, `os.Getenv("INTERNAL_API_KEY")`)
	assert.True(t, aiCallIdx >= 0 && sharedCallIdx >= 0,
		"必须存在 os.Getenv(AI_LLM_INTERNAL_API_KEY) 与 os.Getenv(INTERNAL_API_KEY) 两次调用")
	assert.True(t, aiCallIdx < sharedCallIdx,
		"os.Getenv(\"AI_LLM_INTERNAL_API_KEY\") 必须在 os.Getenv(\"INTERNAL_API_KEY\") 之前")
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
