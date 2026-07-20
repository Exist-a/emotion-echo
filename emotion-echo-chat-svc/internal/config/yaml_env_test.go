package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage 26-P · Commit P3 的 RED 测试:
// chat-api.yaml 必须把 host=localhost 硬编码改为 ${ENV:-容器 DNS 占位},
// 不然 Dockerfile 起容器时会因为 localhost 解析不到 emotion-echo-postgres
// / emotion-echo-kafka / emotion-echo-sw-oap 而失败。
//
// 这些断言在 GREEN 阶段(本 commit)会被 yaml 改动满足。
func TestYaml_HasEnvPlaceholders(t *testing.T) {
	const relYaml = "../../etc/chat-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err, "read chat-api.yaml")
	body := string(raw)

	// Postgres DSN 占位 (默认 fallback 到容器 DNS 名)
	require.Contains(t, body, "${POSTGRES_DSN:-",
		"chat-api.yaml must use ${POSTGRES_DSN:-...} placeholder")
	require.Contains(t, body, "host=emotion-echo-postgres",
		"chat-api.yaml must default DSN to container DNS emotion-echo-postgres")

	// SkyWalking OAPAddr 占位
	require.Contains(t, body, "${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}",
		"chat-api.yaml must use ${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}")

	// Kafka BrokersCSV (list 字段改成 string,符合 ai-svc 范式)
	require.Contains(t, body, "${KAFKA_BROKERS:-",
		"chat-api.yaml must use ${KAFKA_BROKERS:-...} for kafka brokers (list field cannot be env-expanded)")
}

// TestYaml_NoBareLocalhostHost 收尾:不应再出现 host=localhost / Brokers: ["localhost..."]
// 等\"裸 localhost\"硬编码(已 env 占位化后必须消失)
func TestYaml_NoBareLocalhostHost(t *testing.T) {
	const relYaml = "../../etc/chat-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err, "read chat-api.yaml")
	body := string(raw)

	// 干掉 ${VAR:-} 的占位片段后再检测\"硬编码 localhost\"
	stripped := stripEnvPlaceholders(body)
	require.False(t,
		strings.Contains(stripped, `Brokers: ["localhost`),
		"Brokers list must not be a hardcoded localhost (chat-svc must use BrokersCSV via env)",
	)
}

func stripEnvPlaceholders(s string) string {
	// 简单去掉 ${...:-default} 占位
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.Index(s[i:], "}")
			if end >= 0 {
				i += end + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
