package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGoModReplace_SharedModule 静态合同。
//
// user-svc 当前已合规;Stage 26-P · Commit P1 把它收入 4 仓回归保护集合。
func TestGoModReplace_SharedModule(t *testing.T) {
	const relGoMod = "../../go.mod"

	raw, err := os.ReadFile(relGoMod)
	require.NoError(t, err, "read go.mod")

	require.True(t,
		strings.Contains(string(raw), "replace github.com/emotion-echo/shared => ../emotion-echo-shared"),
		"go.mod must include local replace directive for emotion-echo-shared (Dockerfile build will fail otherwise)",
	)
}
