package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage 26-P · Commit P3 的 RED 测试。
func TestYaml_HasEnvPlaceholders(t *testing.T) {
	const relYaml = "../../etc/user-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err, "read user-api.yaml")
	body := string(raw)

	require.Contains(t, body, "${POSTGRES_DSN:-",
		"user-api.yaml must use ${POSTGRES_DSN:-...} placeholders")
	require.Contains(t, body, "host=emotion-echo-postgres",
		"user-api.yaml must default DSN to container DNS emotion-echo-postgres")

	require.Contains(t, body, "${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}",
		"user-api.yaml must use ${SKYWALKING_OAP_ADDR:-emotion-echo-sw-oap:11800}")
}
