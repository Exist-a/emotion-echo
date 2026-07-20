package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGoModReplace_SharedModule 静态合同:
// 所有 emotion-echo-* svc 必须能在容器内通过 `replace` 指令链接本地 shared module,
// 不然 Dockerfile build 阶段 `go mod download` 会 fail。
//
// 这是 Stage 26-P · Commit P1 的 RED 测试:在补 replace 之前 analytics-svc 的
// go.mod 没有这条指令,GREEN 阶段会把它加上。
func TestGoModReplace_SharedModule(t *testing.T) {
	// 仓内相对路径:本测试在 internal/config/ 下,go.mod 在 ../../../../go.mod
	const relGoMod = "../../go.mod"

	raw, err := os.ReadFile(relGoMod)
	require.NoError(t, err, "read go.mod")

	require.True(t,
		strings.Contains(string(raw), "replace github.com/emotion-echo/shared => ../emotion-echo-shared"),
		"go.mod must include local replace directive for emotion-echo-shared (Dockerfile build will fail otherwise)",
	)
}
