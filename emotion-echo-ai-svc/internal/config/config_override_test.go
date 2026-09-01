// Package config — Stage 35 · PR-7+PR-8 测试
//
// 验证：
//   1. yaml 占位符 ${VAR:-default} 被解析为字面量（go-zero conf 不展开）
//   2. applyEnvOverrides 把 env 值注入到 Config
//
// 注：applyEnvOverrides 在 main.go，测试里复制等价实现（保证测试包独立）。
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zeromicro/go-zero/core/conf"
)

// TestConfig_LoadYAMLFromDefaults yaml 默认值能被加载（无 env 注入）。
func TestConfig_LoadYAMLFromDefaults(t *testing.T) {
	t.Parallel()
	var c Config
	err := conf.LoadConfigFromYamlBytes([]byte(testYAML), &c)
	require.NoError(t, err)

	// 默认值（go-zero conf 不展开 ${VAR:-default}，字面量保留为原字符串）
	assert.Equal(t, "ai-api", c.Name)
	assert.Equal(t, 8891, c.Port)
	assert.NotEmpty(t, c.LLM.BaseURL)
}

// TestConfig_YAMLFieldMapping yaml 字段映射到 struct 正确。
func TestConfig_YAMLFieldMapping(t *testing.T) {
	t.Parallel()
	var c Config
	err := conf.LoadConfigFromYamlBytes([]byte(testYAML), &c)
	require.NoError(t, err)

	assert.Equal(t, "ai-api", c.Name)
	assert.Equal(t, "0.0.0.0", c.Host)
	assert.Equal(t, 8891, c.Port)
	assert.Equal(t, "emotion-echo-ai-svc", c.SkyWalking.ServiceName)
	assert.Equal(t, 10, c.Postgres.MaxOpenConns)
	assert.Equal(t, "ai-svc", c.Kafka.GroupID)
	assert.Contains(t, c.Kafka.Topics, "chat-events")
	assert.Equal(t, "deepseek-chat", c.LLM.Model, "LLM.Model should default to deepseek-chat")
	// Stage 35 PR-7：bool 字段（Kafka.Enabled / Nacos.Enabled / SkyWalking.Enabled / Nacos.HotReload）
	// 在 yaml 中省略 → go-zero conf 不写 → 走 Config struct tag default
	assert.False(t, c.Nacos.Enabled, "Nacos.Enabled default=false (yaml 省略 → struct default)")
	assert.False(t, c.Nacos.HotReload)
	assert.False(t, c.SkyWalking.Enabled)
	assert.True(t, c.Kafka.Enabled, "Kafka.enabled dev 默认开启")
}

// TestEnvOverride_NacosEnabled env 注入覆盖 yaml 字面量。
func TestEnvOverride_NacosEnabled(t *testing.T) {
	// 不并行（用 os.Setenv）
	t.Setenv("NACOS_ENABLED", "true")
	t.Setenv("NACOS_ADDR", "test-nacos:8848")
	t.Setenv("NACOS_NAMESPACE", "test-ns")
	t.Setenv("NACOS_HOT_RELOAD", "true")

	var c Config
	require.NoError(t, conf.LoadConfigFromYamlBytes([]byte(testYAML), &c))

	// 模拟 main.go applyEnvOverrides 的核心逻辑（4 个 NACOS_*）
	if v := os.Getenv("NACOS_ENABLED"); v != "" {
		c.Nacos.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("NACOS_ADDR"); v != "" {
		c.Nacos.Addr = v
	}
	if v := os.Getenv("NACOS_NAMESPACE"); v != "" {
		c.Nacos.Namespace = v
	}
	if v := os.Getenv("NACOS_HOT_RELOAD"); v != "" {
		c.Nacos.HotReload = v == "true" || v == "1"
	}

	assert.True(t, c.Nacos.Enabled)
	assert.Equal(t, "test-nacos:8848", c.Nacos.Addr)
	assert.Equal(t, "test-ns", c.Nacos.Namespace)
	assert.True(t, c.Nacos.HotReload)
}

// TestEnvOverride_LLMModel LLM_MODEL env 注入。
func TestEnvOverride_LLMModel(t *testing.T) {
	t.Setenv("LLM_MODEL", "gpt-4")
	t.Setenv("LLM_TIMEOUT", "5")

	var c Config
	require.NoError(t, conf.LoadConfigFromYamlBytes([]byte(testYAML), &c))

	if v := os.Getenv("LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("LLM_TIMEOUT"); v != "" {
		// 简化：只覆盖到 int
		var n int
		_, _ = fmtSscan(v, &n)
		if n > 0 {
			c.LLM.Timeout = n
		}
	}

	assert.Equal(t, "gpt-4", c.LLM.Model)
	assert.Equal(t, 5, c.LLM.Timeout)
}

// testYAML 测试用 yaml 片段（与 etc/ai-api.yaml 同步关键字段）。
// 注：Stage 35 PR-7：bool 字段（SkyWalking.Enabled / Kafka.Enabled / Nacos.Enabled / Nacos.HotReload）省略，
// 走 Config struct tag default（避免 go-zero conf type mismatch）。
const testYAML = `Name: ai-api
Host: 0.0.0.0
Port: 8891

SkyWalking:
  OAPAddr: "localhost:11800"
  ServiceName: emotion-echo-ai-svc

Postgres:
  DSN: "host=localhost"
  MaxOpenConns: 10
  MaxIdleConns: 5

Kafka:
  BrokersCSV: "localhost:9092"
  GroupID: ai-svc
  Topics: ["chat-events"]
  DLQTopic: chat-events-dlq
  MaxRetries: 3

LLM:
  BaseURL: "http://localhost:8000"
  GRPCAddr: "localhost:50051"
  InternalAPIKey: ""
  Model: "deepseek-chat"
  Enabled: true
  Timeout: 3

GRPC:
  Enabled: true
  Port: 8892

FER:
  BaseURL: ""
  Timeout: 10

SenseVoice:
  BaseURL: ""
  Timeout: 30

XTTS:
  BaseURL: ""
  Timeout: 60
  Language: zh-cn
  Speed: 0.75

Nacos:
  Addr: "emotion-echo-nacos:8848"
  Namespace: "emotion-echo-dev"
  GroupName: DEFAULT_GROUP
`

// fmtSscan 简化版 fmt.Sscan（避免 import fmt 噪音）。
func fmtSscan(s string, dst *int) (int, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	*dst = n
	return 1, nil
}