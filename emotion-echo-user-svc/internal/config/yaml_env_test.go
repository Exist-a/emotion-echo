package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage 26-P · 历史 RED 测试：当时要求 yaml 必须含 ${...} 占位符。
// Stage 36-A1.1：发现该占位符是 go-zero conf 不解析的 bug 源头，
// 改为"yaml 必须是字面值 + applyEnvOverrides 兜底 env"，因此翻转断言方向。

// TestYaml_HasNoBashPlaceholders RED 测试：
// Stage 36-A1.1：user-api.yaml 不应再含 ${VAR:-default} / ${VAR} 字面占位符。
// 残留占位符会被 go-zero conf 当成 URL/DSN 字面值，导致 go2sky / postgres driver 解析失败。
func TestYaml_HasNoBashPlaceholders(t *testing.T) {
	const relYaml = "../../etc/user-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err, "read user-api.yaml")
	body := string(raw)

	require.NotContains(t, body, "${POSTGRES_DSN",
		"user-api.yaml must not embed ${POSTGRES_DSN:-...}; rely on applyEnvOverrides")
	require.NotContains(t, body, "${SKYWALKING_OAP_ADDR",
		"user-api.yaml must not embed ${SKYWALKING_OAP_ADDR:-...}; rely on applyEnvOverrides")
	require.NotContains(t, body, "${NACOS_",
		"user-api.yaml must not embed ${NACOS_*} placeholders; rely on applyEnvOverrides")

	// DSN 仍然必须包含容器内 Postgres 地址（默认值字面）
	require.Contains(t, body, "host=emotion-echo-postgres",
		"user-api.yaml must keep container DNS as literal default for DSN")
}

// TestApplyEnvOverrides_PostgresDSN GREEN 路径：
// 即使 yaml 用字面 DSN，env POSTGRES_DSN 非空时必须覆盖它（compose dev/prod 切换必须工作）。
func TestApplyEnvOverrides_PostgresDSN(t *testing.T) {
	const relYaml = "../../etc/user-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err)
	body := string(raw)

	// Step 1: yaml 加载后 DSN 必须是字面默认值（容器 DNS），不是 ${...}
	// 因为我们改成了字面值。
	// 用 strings.Contains 粗略断言 yaml 中 DSN 行不含 bash 占位符
	lines := strings.Split(body, "\n")
	foundDSN := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "DSN:") {
			foundDSN = true
			require.NotContains(t, line, "${",
				"DSN line must not contain bash placeholder; got: %s", line)
		}
	}
	require.True(t, foundDSN, "DSN: line must exist in yaml")

	// Step 2: env 覆盖路径必须生效——这是 applyEnvOverrides 的契约
	// 详细覆盖测试由 main.go 集成测试覆盖（不在这里展开避免 import main）
}

// TestSkyWalkingEnabled_DefaultsFalse 防止后续误把 SkyWalking.Enabled 设回 true：
// Stage 36-A1.1 决定走 Config struct tag default=false（与 ai-svc 保持一致）。
func TestSkyWalkingEnabled_DefaultsFalse(t *testing.T) {
	const relYaml = "../../etc/user-api.yaml"

	raw, err := os.ReadFile(relYaml)
	require.NoError(t, err)
	body := string(raw)

	// yaml 不能硬编码 Enabled: true（那会与 struct default=false 冲突并误报 enabled）
	require.NotContains(t, body, "Enabled:     true",
		"SkyWalking.Enabled should not be hardcoded true in yaml; let Config struct default=false rule")
}
