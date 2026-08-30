package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage 26-P · Commit P3 的 RED 测试。
func TestYaml_HasEnvPlaceholders(t *testing.T) {
	const relYaml = "../../etc/analytics-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err, "read analytics-api.yaml")
	body := string(raw)

	require.Contains(t, body, "${POSTGRES_DSN:-",
		"analytics-api.yaml must use ${POSTGRES_DSN:-...} placeholders")
	require.Contains(t, body, "host=emotion-echo-postgres",
		"analytics-api.yaml must default DSN to container DNS emotion-echo-postgres")

	require.Contains(t, body, "${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}",
		"analytics-api.yaml must use ${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}")

	// Stage 30-B: Kafka consumer 段
	require.Contains(t, body, "${KAFKA_BROKERS:-",
		"analytics-api.yaml must use ${KAFKA_BROKERS:-...} placeholder")
	require.Contains(t, body, "GroupID:    analytics-svc",
		"analytics-api.yaml must set Kafka GroupID to analytics-svc")
	require.Contains(t, body, `Topics:     ["chat-events"]`,
		"analytics-api.yaml must subscribe chat-events")
}
