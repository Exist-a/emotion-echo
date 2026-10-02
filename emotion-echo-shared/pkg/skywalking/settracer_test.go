// Package skywalking — E2E-26：SetTracer 注册全局 tracer 的契约测试
//
// 根因（2026-10-02 E2E-26 执行期实测）：本包的全局 tracer 只有 Init() 一条
// 初始化路径，而 **全仓零调用**（grep `skywalking.Init()` 除注释外无命中），
// Init() 还读已废弃的 env（SKY_SW_OAP_ADDR，compose 实际设的是
// SKYWALKING_OAP_ADDR）⇒ `skywalking.Tracer()` 恒 nil ⇒ 5 个 svc 的
// gRPC server 拦截器（ai/analytics/assessment/chat/user 均取全局 tracer）
// 全部拿到 typed-nil → noop ⇒ **gRPC 服务端 span 从未上报、sw8 从不被提取**。
// 各 svc main.go 已有 BootstrapSkyWalkingTracer 成功分支，缺的只是把它
// 注册进本包的一步 —— SetTracer 即该契约。
package skywalking

import (
	"testing"

	"github.com/SkyAPM/go2sky"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetTracer_RegistersGlobalTracer(t *testing.T) {
	// 独立测试不依赖其他 case 的全局状态：先清
	SetTracer(nil)
	require.Nil(t, Tracer(), "SetTracer(nil) 后 Tracer() 应为 nil")

	tr, err := go2sky.NewTracer("settracer-test")
	require.NoError(t, err)
	require.NotNil(t, tr)

	SetTracer(tr)
	assert.Same(t, tr, Tracer(), "SetTracer 后 Tracer() 必须返回同一实例")

	// 可清回（测试隔离 + 未来热更新）
	SetTracer(nil)
	assert.Nil(t, Tracer())
}
