// Package skywalking — E2E-26：SetTracer 装配守卫（5 svc + web-bff）
//
// 取全局 tracer 构造 gRPC server 拦截器 / 出站埋点的服务，必须在 main() 的
// BootstrapSkyWalkingTracer 成功分支调用 SetTracer(t)，否则拿到 typed-nil tracer
// → noop → span 与 sw8 提取静默失效。静态源断言：删掉任一调用 → 本测试 FAIL。
//
// CL-01 D-04：新增 web-bff（此前只有局部 packageTracer，不在守卫覆盖内 = 盲区）。
// BFF 用**无 gorm 依赖**的 pkg/skywalking/trace（避免把 ORM 拖进网关），故调用串不同。
package skywalking

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGRPCSvcMains_CallSetTracer 5 个 svc + web-bff 的 main.go 必须注册 SetTracer。
func TestGRPCSvcMains_CallSetTracer(t *testing.T) {
	svcs := []struct {
		dir     string
		setCall string // 该 main.go 里应出现的确切调用串
	}{
		{"emotion-echo-user-svc", "skywalking.SetTracer("},
		{"emotion-echo-chat-svc", "skywalking.SetTracer("},
		{"emotion-echo-ai-svc", "skywalking.SetTracer("},
		{"emotion-echo-assessment-svc", "skywalking.SetTracer("},
		{"emotion-echo-analytics-svc", "skywalking.SetTracer("},
		// web-bff 用无 gorm 依赖的 trace 包（CL-01 D-04）
		{"emotion-echo-web-bff", "sharedtrace.SetTracer("},
	}
	for _, tc := range svcs {
		tc := tc
		t.Run(tc.dir, func(t *testing.T) {
			// 本包位于 <repo>/emotion-echo-shared/pkg/skywalking → 仓库根 = 三级之上
			path := filepath.Join("..", "..", "..", tc.dir, "main.go")
			src, err := os.ReadFile(path)
			require.NoError(t, err, "读 %s 失败（路径锚点变了需同步）", path)
			body := string(src)

			assert.Contains(t, body, tc.setCall,
				"%s/main.go 必须在 tracer 初始化成功分支调用 %s —— "+
					"取全局 tracer 的 gRPC server 拦截器 / 出站埋点不注册则恒 nil（E2E-26 #3 根因）",
				tc.dir, tc.setCall)

			// 顺序：SetTracer 必须发生在 tracer bootstrap 之后（注册成功产物）
			// 锚点用**不带括号**的前缀，兼容 `BootstrapSkyWalkingTracer(` 与
			// `BootstrapSkyWalkingTracerRetry(`（E2E-F-204 引入的重试变体）。
			bootIdx := strings.Index(body, "BootstrapSkyWalkingTracer")
			setIdx := strings.Index(body, tc.setCall)
			require.GreaterOrEqual(t, bootIdx, 0, "%s 必须用 BootstrapSkyWalkingTracer[Retry]", tc.dir)
			assert.Greater(t, setIdx, bootIdx,
				"%s 的 SetTracer 必须在 BootstrapSkyWalkingTracer 之后（只注册初始化成功的 tracer）", tc.dir)
		})
	}
}
