// Package main — E2E-26 测试点 #3 前置守卫：SkyWalking tracer 初始化必须先于下游 gRPC 拨号
//
// 根因（2026-10-02 E2E-26 执行期实测）：main() 里 `svcCtx := buildServiceContext(...)`
// 排在 "// 2. SkyWalking" 初始化块**之前**，而 buildServiceContext →
// `ClientDialOptions(NewGo2SkyTracer(packageTracer), ...)` 在拨号时读 packageTracer。
// 顺序颠倒 ⇒ packageTracer 恒 nil ⇒ 全部下游 gRPC conn 不挂 tracing interceptor
// ⇒ BFF 从不注入 sw8，跨进程 trace 在 BFF→user/chat/... 第一跳就断。
// 单测全绿（mock 注入 tracer）掩盖了 main() 的真实装配顺序 —— 本测试钉死顺序。
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMain_SkyWalkingInit_BeforeBuildServiceContext tracer 初始化（packageTracer 赋值）
// 必须发生在 buildServiceContext 调用之前。
func TestMain_SkyWalkingInit_BeforeBuildServiceContext(t *testing.T) {
	src, err := os.ReadFile("main.go")
	assert.NoError(t, err)
	body := string(src)

	// 调用点（不是函数定义）：main() 内 `svcCtx := buildServiceContext(`
	callIdx := strings.Index(body, "svcCtx := buildServiceContext(")
	assert.GreaterOrEqual(t, callIdx, 0,
		"main() 必须存在 `svcCtx := buildServiceContext(` 调用点（锚点变了需同步本测试）")

	// packageTracer 赋值点必须在调用点之前
	tracerIdx := strings.Index(body, "packageTracer = t")
	assert.GreaterOrEqual(t, tracerIdx, 0,
		"main() 必须存在 `packageTracer = t` 赋值（tracer 初始化成功分支）")

	assert.Less(t, tracerIdx, callIdx,
		"SkyWalking tracer 初始化（packageTracer = t）必须先于 buildServiceContext 调用——"+
			"否则 ClientDialOptions 拿到 nil tracer，BFF 下游 gRPC 全部不注入 sw8（E2E-26 #3 根因）")
}
