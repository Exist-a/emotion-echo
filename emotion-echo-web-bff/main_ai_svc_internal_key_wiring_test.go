package main

// CL-F-66：BFF → ai-svc gRPC 必须携带 x-internal-api-key。
//
// 用源码级钉（与 main_internal_api_key_wiring_test.go 同风格）：
// ① main.go 必须读取 per-svc env BFF_AI_SVC_INTERNAL_API_KEY（fallback INTERNAL_API_KEY）；
// ② 必须使用 shared 的 NewClientAuthInterceptor 装配到 ai-svc 的 dial options 上。

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain_AISvcInternalAPIKeyWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	require.NoError(t, err)
	body := string(src)

	assert.Contains(t, body, "BFF_AI_SVC_INTERNAL_API_KEY",
		"main.go 必须支持 per-svc env BFF_AI_SVC_INTERNAL_API_KEY（最小权限：与 BFF_LLM_INTERNAL_API_KEY 同风格）")
	assert.Contains(t, body, "NewClientAuthInterceptor",
		"main.go 必须用 grpcinterceptor.NewClientAuthInterceptor 给 ai-svc 连接挂内部鉴权")
	assert.Contains(t, body, "aiDialOpts",
		"ai-svc 的两条连接（AIClient / EmotionQ+Memory）必须走带 key 的 dial options")
}
