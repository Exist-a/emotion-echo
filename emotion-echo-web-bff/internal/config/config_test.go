// Package config — config_test.go
//
// Stage 30 / stage-30-web-bff.md T1.1 RED:
// 断言 config yaml parsing 正确加载默认值 + 下游地址合法。
//
// 跑：go test ./internal/config/...
package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"

	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
)

// loadTestConfig 从 etc/web-bff.yaml 加载（测试用）
func loadTestConfig(t *testing.T) Config {
	t.Helper()
	var c Config
	err := conf.Load(filepath.Join("..", "..", "etc", "web-bff.yaml"), &c)
	require.NoError(t, err, "web-bff.yaml 应可被 go-zero conf 加载")
	return c
}

// TestConfig_YamlParsing_Port8894 断言 Port=8894（BFF 服务端口）
func TestConfig_YamlParsing_Port8894(t *testing.T) {
	c := loadTestConfig(t)
	assert.Equal(t, 8894, c.Port, "BFF 端口应 8894")
	assert.Equal(t, "0.0.0.0", c.Host)
	assert.Equal(t, shareddiscovery.ServiceWebBFF, c.Name)
}

// TestConfig_DownstreamDefaults_Valid 断言 5 个下游默认地址非空且端口合法
func TestConfig_DownstreamDefaults_Valid(t *testing.T) {
	c := loadTestConfig(t)

	// 5 个下游 HTTP
	assert.NotEmpty(t, c.UserService.BaseURL, "user-svc BaseURL 应有默认值")
	assert.NotEmpty(t, c.ChatService.BaseURL, "chat-svc BaseURL 应有默认值")
	assert.NotEmpty(t, c.AssessmentService.BaseURL, "assessment-svc BaseURL 应有默认值")
	assert.NotEmpty(t, c.AnalyticsService.BaseURL, "analytics-svc BaseURL 应有默认值")

	// AI 服务双协议
	assert.NotEmpty(t, c.AIService.HTTPAddr, "ai-svc HTTP 地址应有默认值")
	assert.NotEmpty(t, c.AIService.GRPCAddr, "ai-svc gRPC 地址应有默认值")

	// XTTS
	assert.NotEmpty(t, c.XTTS.BaseURL, "XTTS BaseURL 应有默认值")

	// 超时默认值 > 0
	assert.Greater(t, c.UserService.TimeoutMs, 0)
	assert.Greater(t, c.AIService.TimeoutMs, 0)
	assert.Greater(t, c.Health.TimeoutMs, 0)
}

// TestConfig_YamlParsing_AIServiceGRPCAddr 断言 AIService.GRPCAddr 默认值合法
// （T1.1 文档要求：断言 AIService.GRPCAddr 默认值合法）
func TestConfig_YamlParsing_AIServiceGRPCAddr(t *testing.T) {
	c := loadTestConfig(t)
	assert.Equal(t, "localhost:8892", c.AIService.GRPCAddr, "默认 gRPC 地址应为 localhost:8892")
	assert.Equal(t, "http://localhost:8891", c.AIService.HTTPAddr, "默认 HTTP 地址应为 localhost:8891")
}

// TestConfig_ApplyEnvOverrides_OverridesDefaults T1.3 REFACTOR:
// 容器 env 注入后应覆盖 yaml 默认值（指向容器 DNS）。
func TestConfig_ApplyEnvOverrides_OverridesDefaults(t *testing.T) {
	c := loadTestConfig(t)

	t.Setenv("USER_SVC_URL", "http://emotion-echo-user-svc:8888")
	t.Setenv("CHAT_SVC_URL", "http://emotion-echo-chat-svc:8890")
	t.Setenv("ASSESSMENT_SVC_URL", "http://emotion-echo-assessment-svc:8889")
	t.Setenv("ANALYTICS_SVC_URL", "http://emotion-echo-analytics-svc:8904")
	t.Setenv("AI_SVC_HTTP_URL", "http://emotion-echo-ai-svc:8891")
	t.Setenv("AI_SVC_GRPC_ADDR", "emotion-echo-ai-svc:8892")
	t.Setenv("XTTS_BASE_URL", "http://emotion-echo-xtts:8003")
	t.Setenv("SKYWALKING_OAP_ADDR", "emotion-echo-sw-oap:11800")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "http://emotion-echo-user-svc:8888", c.UserService.BaseURL)
	assert.Equal(t, "http://emotion-echo-chat-svc:8890", c.ChatService.BaseURL)
	assert.Equal(t, "http://emotion-echo-assessment-svc:8889", c.AssessmentService.BaseURL)
	assert.Equal(t, "http://emotion-echo-analytics-svc:8904", c.AnalyticsService.BaseURL)
	assert.Equal(t, "http://emotion-echo-ai-svc:8891", c.AIService.HTTPAddr)
	assert.Equal(t, "emotion-echo-ai-svc:8892", c.AIService.GRPCAddr)
	assert.Equal(t, "http://emotion-echo-xtts:8003", c.XTTS.BaseURL)
	assert.Equal(t, "emotion-echo-sw-oap:11800", c.SkyWalking.OAPAddr)
}

// TestConfig_ApplyEnvOverrides_EmptyEnv_KeepsDefault 空 env 不应覆盖默认值
func TestConfig_ApplyEnvOverrides_EmptyEnv_KeepsDefault(t *testing.T) {
	c := loadTestConfig(t)
	t.Setenv("USER_SVC_URL", "") // 空串视为未设置

	ApplyEnvOverrides(&c)

	assert.Equal(t, "http://localhost:8888", c.UserService.BaseURL, "空 env 不应覆盖默认值")
}
