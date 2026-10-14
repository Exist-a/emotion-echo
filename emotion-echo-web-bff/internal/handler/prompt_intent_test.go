// Package handler — prompt_intent_test.go
//
// CL-04 · Phase 5（T-11）：**意图风格注入接回**（死路径复活的防腐钉）。
//
// 缺陷 5（plan §0.2）：`llm-service` 的 `ChatCompletion` 只在 `with_intent=True` 时
// `inject_style(...)`，而 BFF 从不设置该字段（`llm_grpc.go` 组 pb 时不带）
// ⇒ 整条"按意图调整回复风格"的链路**从未在生产触发**（死路径）。
//
// 处置（roadmap §4 CL-04「已定参数」）：**接回** —— BFF 下发 `with_intent=true`。
//
// 成本提示（写进 report）：llm-service 侧规则式拿不准时会用 LLM 消歧（≤5s，在首字之前）
// ⇒ 本阶段收口须实测 TTFT 与基线对比。
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-11a：Handler 必须向上游声明"要按意图注入风格"
func TestIntentInjection_HandlerRequestsWithIntent(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"好"}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(config.Config{}, AIStreamDeps{LLM: fake}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		strings.NewReader(`{"message":"我有点焦虑","conversationId":"0"}`))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, fake.gotWithIntent,
		"BFF 必须下发 with_intent=true，否则 llm-service 的 inject_style 永不触发（缺陷 5 死路径）")
}

// T-11b：开关是**单一来源**常量（将来要回滚/灰度时只有一个切换点）
func TestIntentInjection_SwitchIsSingleSource(t *testing.T) {
	assert.True(t, llmWithIntent, "CL-04 Phase 5 的处置是「接回」；若改为不接回须同步改测试与文档")
	var req downstream.LLMStreamRequest
	req.WithIntent = llmWithIntent
	assert.True(t, req.WithIntent)
}
