package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage 26-P · 历史 RED 测试：当时要求 yaml 必须含 ${...} 占位符。
// Stage 36-A1.3：发现该占位符是 go-zero conf 不解析的 bug 源头，
// 改为"yaml 必须是字面值 + applyEnvOverrides 兜底 env"，翻转断言方向。

// TestYaml_HasNoBashPlaceholders RED 测试：
// analytics-api.yaml 非注释行不应再含 ${VAR:-default} / ${VAR} 字面占位符。
// 残留占位符会被 go-zero conf 当成 host/DSN 字面值传给 go2sky / postgres driver / sarama。
func TestYaml_HasNoBashPlaceholders(t *testing.T) {
	const relYaml = "../../etc/analytics-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err, "read analytics-api.yaml")

	nonCommentLines := 0
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		nonCommentLines++
		require.NotContains(t, line, "${",
			"non-comment line must not contain bash placeholder: %s", line)
	}
	require.Greater(t, nonCommentLines, 0, "yaml must have non-comment config lines")

	// DSN 仍然必须包含容器内 Postgres 地址（默认值字面）
	require.Contains(t, string(raw), "host=emotion-echo-postgres",
		"analytics-api.yaml must keep container DNS as literal default for DSN")

	// Kafka 关键字段必须保留（功能依赖）
	require.Contains(t, string(raw), "GroupID:    analytics-svc",
		"analytics-api.yaml must set Kafka GroupID to analytics-svc")
	require.Contains(t, string(raw), `Topics:     ["chat-events"]`,
		"analytics-api.yaml must subscribe chat-events")
}

// TestKafkaEnabled_DefaultsTrue analytics-svc 依赖 Kafka consumer 写入 user_behavior_events
// （Stage 30-B），Kafka.Enabled 走 Config struct tag default=true。
func TestKafkaEnabled_DefaultsTrue(t *testing.T) {
	const relYaml = "../../etc/analytics-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err)
	body := string(raw)

	require.NotContains(t, body, "Enabled:    true",
		"Kafka.Enabled should not be hardcoded true in yaml; let Config struct default=true rule (consumer is the only data source for analytics-svc)")
}
