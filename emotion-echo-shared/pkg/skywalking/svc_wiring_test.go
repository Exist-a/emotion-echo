// Package skywalking — E2E-26：5 个 gRPC 服务的 SetTracer 装配守卫
//
// 取全局 tracer（skywalking.Tracer()）构造 gRPC server 拦截器的 5 个 svc
// 必须在 main() 的 BootstrapSkyWalkingTracer 成功分支调用 SetTracer(t)，
// 否则 gRPC server 拿 typed-nil tracer → noop → 服务端 span/ sw8 提取全灭。
// 静态源断言（仓内 main_*_wiring_test 同范式）：删掉任一调用 → 本测试 FAIL。
package skywalking

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGRPCSvcMains_CallSetTracer 5 个取全局 tracer 的 svc main.go 必须注册 SetTracer。
func TestGRPCSvcMains_CallSetTracer(t *testing.T) {
	svcs := []string{
		"emotion-echo-user-svc",
		"emotion-echo-chat-svc",
		"emotion-echo-ai-svc",
		"emotion-echo-assessment-svc",
		"emotion-echo-analytics-svc",
	}
	for _, svc := range svcs {
		t.Run(svc, func(t *testing.T) {
			// 本包位于 <repo>/emotion-echo-shared/pkg/skywalking → 仓库根 = 三级之上
			path := filepath.Join("..", "..", "..", svc, "main.go")
			src, err := os.ReadFile(path)
			require.NoError(t, err, "读 %s 失败（路径锚点变了需同步）", path)
			body := string(src)

			assert.Contains(t, body, "skywalking.SetTracer(",
				"%s/main.go 必须在 tracer 初始化成功分支调用 skywalking.SetTracer(t)——"+
					"该 svc 的 gRPC server 拦截器取全局 skywalking.Tracer()，不注册则恒 nil（E2E-26 #3 根因）", svc)

			// 顺序：SetTracer 必须发生在 BootstrapSkyWalkingTracer 之后（注册成功产物）
			bootIdx := strings.Index(body, "BootstrapSkyWalkingTracer(")
			setIdx := strings.Index(body, "skywalking.SetTracer(")
			require.GreaterOrEqual(t, bootIdx, 0, "%s 必须用 BootstrapSkyWalkingTracer", svc)
			assert.Greater(t, setIdx, bootIdx,
				"%s 的 SetTracer 必须在 BootstrapSkyWalkingTracer 之后（只注册初始化成功的 tracer）", svc)
		})
	}
}
