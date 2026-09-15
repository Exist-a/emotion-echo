// Package main — Round 4.6 P2-25 silent fallback 收紧测试
//
// 锁住：
// 1. APP_ENV=prod 时 applyDefaultFallbacks 不覆盖任何字段
// 2. dev 模式即使基础设施（PG/Kafka）保留 localhost 默认，业务依赖字段
//    （LLM.BaseURL/GRPCAddr/SkyWalking.OAPAddr）也必须显式配置，否则返 error
// 3. main.go 必须 fail-fast（os.Exit(1)）如果 applyDefaultFallbacks 返 error
//
// 否则 prod 误配会让 LLM 请求打到 dev mock（silent localhost fallback）。
package main

import (
	"os"
	"testing"

	emotionconfig "emotion-echo-ai-svc/internal/config"

	sharedconfig "github.com/emotion-echo/shared/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyDefaultFallbacks_DevProfile_InfrastructureOnlyLocalhost dev 模式保留基础设施 localhost 默认
// （PG / Kafka 这些 dev docker-compose 部署必备）
func TestApplyDefaultFallbacks_DevProfile_InfrastructureOnlyLocalhost(t *testing.T) {
	os.Unsetenv("APP_ENV")

	c := &emotionconfig.Config{
		// 业务依赖字段必须显式给（dev 也不再 silently localhost fallback）
		LLM:        emotionconfig.LLM{BaseURL: "http://test-llm:8000", GRPCAddr: "test-llm:50051"},
		SkyWalking: emotionconfig.SkyWalking{OAPAddr: "test-oap:11800"},
	}
	err := applyDefaultFallbacks(c)
	require.NoError(t, err, "dev 模式给齐字段应成功")

	assert.Equal(t, "localhost:9092", c.Kafka.BrokersCSV, "dev 模式应填 localhost Kafka 默认")
	assert.Contains(t, c.Postgres.DSN, "host=localhost", "dev 模式应填 localhost Postgres")
	// 业务依赖字段不应被覆盖（已显式给）
	assert.Equal(t, "http://test-llm:8000", c.LLM.BaseURL)
	assert.Equal(t, "test-llm:50051", c.LLM.GRPCAddr)
	assert.Equal(t, "test-oap:11800", c.SkyWalking.OAPAddr)
}

// TestApplyDefaultFallbacks_DevProfile_LLMBaseURLEmpty_FailsFast 业务依赖字段缺失必须 fail-fast
// （修复 verifier 指出"dev 模式也仍含 localhost 默认"）
func TestApplyDefaultFallbacks_DevProfile_LLMBaseURLEmpty_FailsFast(t *testing.T) {
	os.Unsetenv("APP_ENV")

	c := &emotionconfig.Config{} // LLM.BaseURL 空
	err := applyDefaultFallbacks(c)
	assert.Error(t, err, "dev 模式 LLM.BaseURL 空必须返 error（防止 silent localhost fallback）")
	assert.Contains(t, err.Error(), "LLM.BaseURL")
}

// TestApplyDefaultFallbacks_DevProfile_LLMGRPCAddrEmpty_FailsFast
func TestApplyDefaultFallbacks_DevProfile_LLMGRPCAddrEmpty_FailsFast(t *testing.T) {
	os.Unsetenv("APP_ENV")

	c := &emotionconfig.Config{
		LLM: emotionconfig.LLM{BaseURL: "http://x:8000"}, // GRPCAddr 空
	}
	err := applyDefaultFallbacks(c)
	assert.Error(t, err, "dev 模式 LLM.GRPCAddr 空必须返 error")
	assert.Contains(t, err.Error(), "LLM.GRPCAddr")
}

// TestApplyDefaultFallbacks_DevProfile_SkyWalkingOAPAddrEmpty_FailsFast
func TestApplyDefaultFallbacks_DevProfile_SkyWalkingOAPAddrEmpty_FailsFast(t *testing.T) {
	os.Unsetenv("APP_ENV")

	c := &emotionconfig.Config{
		LLM:        emotionconfig.LLM{BaseURL: "http://x:8000", GRPCAddr: "x:50051"},
		SkyWalking: emotionconfig.SkyWalking{}, // OAPAddr 空
	}
	err := applyDefaultFallbacks(c)
	assert.Error(t, err, "dev 模式 SkyWalking.OAPAddr 空必须返 error")
	assert.Contains(t, err.Error(), "SkyWalking.OAPAddr")
}

// TestApplyDefaultFallbacks_ProdProfile_DoesNotTouchFields APP_ENV=prod 不覆盖任何字段
func TestApplyDefaultFallbacks_ProdProfile_DoesNotTouchFields(t *testing.T) {
	os.Setenv("APP_ENV", "prod")
	defer os.Unsetenv("APP_ENV")

	c := &emotionconfig.Config{}
	err := applyDefaultFallbacks(c)
	require.NoError(t, err, "prod 模式只返 nil，不动字段")

	assert.Empty(t, c.Postgres.DSN, "prod 模式不应填 localhost Postgres DSN")
	assert.Empty(t, c.Kafka.BrokersCSV, "prod 模式不应填 localhost Kafka")
	assert.Empty(t, c.SkyWalking.OAPAddr, "prod 模式不应填 localhost SkyWalking")
	assert.Empty(t, c.LLM.BaseURL, "prod 模式不应填 localhost LLM BaseURL")
	assert.Empty(t, c.LLM.GRPCAddr, "prod 模式不应填 localhost LLM GRPCAddr")
}

// TestMain_AppEnvProdGuard_CallerWiring main.go 必须读 APP_ENV（Round 4.6 收紧）
func TestMain_AppEnvProdGuard_CallerWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(src)
	assert.Contains(t, body, "APP_ENV",
		"ai-svc main.go applyDefaultFallbacks 必须读 APP_ENV（Round 4.6 prod fallback 收紧）")
	assert.Contains(t, body, `"prod"`,
		"必须显式比对 prod 字符串（不能用 1/true 等其他 truthy）")
	assert.Contains(t, body, `os.Getenv("APP_ENV") == "prod"`,
		"prod guard 必须是 os.Getenv(\"APP_ENV\") == \"prod\"")
	// Round 4.6 verifier 跟进：applyDefaultFallbacks 返 error 必须 fail-fast
	assert.Contains(t, body, "applyDefaultFallbacks(&c); err != nil",
		"main.go 必须检查 applyDefaultFallbacks 返 error 并 os.Exit(1)")
	assert.Contains(t, body, "os.Exit(1)",
		"fail-fast 必须用 os.Exit(1)")
}

// 防 unused import
var _ = sharedconfig.LoadBytes
