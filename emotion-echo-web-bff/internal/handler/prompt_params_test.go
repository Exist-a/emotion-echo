// Package handler — prompt_params_test.go
//
// CL-04 · Phase 3（A 轨，2026-10-14）：**生成参数真下发**（T-7）。
//
// 缺陷 3：handler 从不给 `LLMStreamRequest` 赋 Temperature / MaxTokens ⇒ 上游用零值
// ⇒ **输出长度无上限**（对"2-3 句话"的人设与成本都不受控）。
//
// 用户 2026-10-14 裁定（plan §2 T-16 [M]）：`Temperature=0.7`、`MaxTokens=1024`。
// 本文件钉三件事：
//   T-7a 两条上游路径都下发参数（gRPC 路径行为断言）
//   T-7b HTTP 直连路径也带（源码断言 —— 防"只修了一条路径"的路径漂移复发）
//   T-7c 参数真能进 wire（`LLMChatReq` 序列化出现 temperature / max_tokens）
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-7a：gRPC 路径必须把裁定值下发到 LLMStreamRequest
func TestLLMParams_SentOnGRPCPath(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"好"}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(config.Config{}, AIStreamDeps{LLM: fake}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		strings.NewReader(`{"message":"你好","conversationId":"0"}`))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0.7, fake.gotTemperature, "Temperature 必须按用户裁定下发（0.7）")
	assert.Equal(t, int32(1024), fake.gotMaxTokens, "MaxTokens 必须按用户裁定下发（1024）")
	assert.NotZero(t, fake.gotTemperature, "零值即缺陷 3 复发")
	assert.NotZero(t, fake.gotMaxTokens, "零值即缺陷 3 复发（上游将无输出长度上限）")
}

// T-7b：两条上游路径**都**带参数（防只修一条 ⇒ 路径漂移）
func TestLLMParams_BothUpstreamPathsCarryThem(t *testing.T) {
	src, err := os.ReadFile("ai_stream_handler.go")
	require.NoError(t, err)
	text := string(src)

	assert.Equal(t, 2, strings.Count(text, "llmGeneration.Temperature"),
		"gRPC 路径与 HTTP 直连路径必须都下发 Temperature")
	assert.Equal(t, 2, strings.Count(text, "llmGeneration.MaxTokens"),
		"gRPC 路径与 HTTP 直连路径必须都下发 MaxTokens（计数不依赖 gofmt 对齐空格）")
}

// T-7c：参数真能进 wire（LLMChatReq 直接 marshal 成上游 body）
func TestLLMParams_SerializedIntoUpstreamBody(t *testing.T) {
	payload, err := json.Marshal(downstream.LLMChatReq{
		Model:       "deepseek-chat",
		Messages:    []downstream.Message{{Role: "user", Content: "hi"}},
		Stream:      true,
		Temperature: 0.7,
		MaxTokens:   1024,
	})
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"temperature":0.7`)
	assert.Contains(t, string(payload), `"max_tokens":1024`)
}
