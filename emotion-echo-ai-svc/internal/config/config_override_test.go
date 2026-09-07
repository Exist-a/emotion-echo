// Package config — Stage 41 PR-5 · ai-svc 配置加载测试
//
// 验证 ai-svc 改用 shared/pkg/config 后语义保持：
//   1. yaml 显式 false 必须保留(R3 反向)
//   2. env 注入(applyEnvOverrides 复制实现)能覆盖 yaml 字面量
//   3. SetDefaults 填零值字段
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedconfig "github.com/emotion-echo/shared/pkg/config"
)

// TestConfig_LoadYAMLFromDefaults yaml 默认值能被加载(无 env 注入)。
func TestConfig_LoadYAMLFromDefaults(t *testing.T) {
	t.Parallel()
	var c Config
	err := sharedconfig.LoadBytes([]byte(testYAML), &c, func() { SetDefaults(&c) })
	require.NoError(t, err)

	assert.Equal(t, "ai-api", c.Name)
	assert.Equal(t, 8891, c.Port)
	assert.NotEmpty(t, c.LLM.BaseURL)
}

// TestConfig_YAMLFieldMapping yaml 字段映射到 struct 正确。
//
// Stage 41 PR-5:R3 反向钉死。yaml 显式 false 必须保留(SetDefaults 不能强制覆盖回 true)。
// ai-svc 复杂:Go bool 零值=false 与"显式 false"无法区分,所以 SetDefaults
// 不填 Kafka.Enabled / LLM.Enabled / Nacos.Enabled / GRPCServer.Enabled 等 bool。
// 这些字段由 yaml 显式给值或 main.go applyEnvOverrides 通过 env 注入。
func TestConfig_YAMLFieldMapping(t *testing.T) {
	t.Parallel()
	var c Config
	err := sharedconfig.LoadBytes([]byte(testYAML), &c, func() { SetDefaults(&c) })
	require.NoError(t, err)

	assert.Equal(t, "ai-api", c.Name)
	assert.Equal(t, "0.0.0.0", c.Host)
	assert.Equal(t, 8891, c.Port)
	assert.Equal(t, "emotion-echo-ai-svc", c.SkyWalking.ServiceName)
	assert.Equal(t, 10, c.Postgres.MaxOpenConns)
	assert.Equal(t, "ai-svc", c.Kafka.GroupID)
	assert.Contains(t, c.Kafka.Topics, "chat-events")
	assert.Equal(t, "deepseek-chat", c.LLM.Model, "LLM.Model should default to deepseek-chat")

	// Stage 35 PR-7:yaml 显式给 false 必须保留(SetDefaults 不能强制覆盖)
	assert.False(t, c.Nacos.Enabled, "yaml Nacos.Enabled: false 必须保留")
	assert.False(t, c.Nacos.HotReload, "yaml Nacos.HotReload: false 必须保留")
	assert.False(t, c.SkyWalking.Enabled, "yaml SkyWalking.Enabled: false 必须保留")
	assert.True(t, c.Kafka.Enabled, "yaml Kafka.Enabled: true 必须保留")
	assert.True(t, c.LLM.Enabled, "yaml LLM.Enabled: true 必须保留")
	assert.True(t, c.GRPC.Enabled, "yaml GRPC.Enabled: true 必须保留")
}

// TestEnvOverride_NacosEnabled env 注入覆盖 yaml 字面量。
func TestEnvOverride_NacosEnabled(t *testing.T) {
	t.Setenv("NACOS_ENABLED", "true")
	t.Setenv("NACOS_ADDR", "test-nacos:8848")
	t.Setenv("NACOS_NAMESPACE", "test-ns")
	t.Setenv("NACOS_HOT_RELOAD", "true")

	var c Config
	require.NoError(t, sharedconfig.LoadBytes([]byte(testYAML), &c, func() { SetDefaults(&c) }))

	// 模拟 main.go applyEnvOverrides 的核心逻辑(4 个 NACOS_*)
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
	require.NoError(t, sharedconfig.LoadBytes([]byte(testYAML), &c, func() { SetDefaults(&c) }))

	if v := os.Getenv("LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("LLM_TIMEOUT"); v != "" {
		var n int
		_, _ = fmtSscan(v, &n)
		if n > 0 {
			c.LLM.Timeout = n
		}
	}

	assert.Equal(t, "gpt-4", c.LLM.Model)
	assert.Equal(t, 5, c.LLM.Timeout)
}

// testYAML 测试用 yaml 片段(与 etc/ai-api.yaml 同步关键字段)。
//
// Stage 41 PR-5 改动:bool 字段全部显式给值(不能省略),因为 SetDefaults 不填 bool。
// yaml 省略 bool 字段会得到零值 false,R3 反向测试需要明确区分"未设置"与"显式 false"。
const testYAML = `Name: ai-api
Host: 0.0.0.0
Port: 8891

SkyWalking:
  OAPAddr: "localhost:11800"
  ServiceName: emotion-echo-ai-svc
  Enabled: false

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
  Enabled: true

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
  Enabled: false
  HotReload: false
`

// fmtSscan 简化版 fmt.Sscan(避免 import fmt 噪音)。
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
