package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGoModReplace_SharedModule 静态合同:所有 emotion-echo-* svc 必须能在容器内
// 通过 `replace` 指令链接本地 shared module,不然 Dockerfile build 阶段
// `go mod download` 会 fail。
//
// Stage 26-P · Commit P1 的回归保护(chat-svc 当前已合规,作为 contract 锁)。
func TestGoModReplace_SharedModule(t *testing.T) {
	const relGoMod = "../../go.mod"

	raw, err := os.ReadFile(relGoMod)
	require.NoError(t, err, "read go.mod")

	require.True(t,
		strings.Contains(string(raw), "replace github.com/emotion-echo/shared => ../emotion-echo-shared"),
		"go.mod must include local replace directive for emotion-echo-shared (Dockerfile build will fail otherwise)",
	)
}
