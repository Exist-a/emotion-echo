// Package handler — ai_stream_handler_llmgrpc_test.go
//
// Stage 81 RED（llm-chat-real-pipeline PR-2）：AIStreamHandler 的上游优先级契约
//   1. llm-service gRPC ChatCompletion 可用 → 流式透传 delta（SSE），不用 mock
//   2. gRPC 传输失败 → 回落既有 Phase D HTTP 直连（有 key 时）或 mock
// llm-service 自身无 key/上游失败在 PR-1 已降级为 mock chunk（fallback_reason），
// BFF 只做透传，不复制降级逻辑。
package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLMStreamer 实现 downstream.LLMChatStreamer
type fakeLLMStreamer struct {
	deltas     []string
	err        error
	gotModel   string
	gotMsgs    []downstream.Message
}

func (f *fakeLLMStreamer) StreamChat(req downstream.LLMStreamRequest, onDelta func(string)) error {
	f.gotModel = req.Model
	f.gotMsgs = req.Messages
	if f.err != nil {
		return f.err
	}
	for _, d := range f.deltas {
		onDelta(d)
	}
	return nil
}

func TestAIStreamHandler_LLMGRPCUpstream_StreamsDeltas(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"真实", "LLM", "回复"}}
	h := &AIStreamHandler{cfg: config.Config{}, llm: fake}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"今天有点累","emotion":"neutral","conversationId":"1"}`))
	h.ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, "真实", "gRPC 上游 delta 必须透传")
	assert.Contains(t, body, "LLM")
	assert.Contains(t, body, "回复")
	assert.Contains(t, body, "data: [DONE]")
	assert.NotContains(t, body, "抱抱你", "上游可用时不得输出 mock 话术")
	// 消息组装：system 人设 + user 内容
	require.NotEmpty(t, fake.gotMsgs)
	assert.Equal(t, "system", fake.gotMsgs[0].Role)
	assert.Equal(t, "今天有点累", fake.gotMsgs[len(fake.gotMsgs)-1].Content)
}

func TestAIStreamHandler_LLMGRPCError_FallsBackToMock(t *testing.T) {
	fake := &fakeLLMStreamer{err: assert.AnError}
	h := &AIStreamHandler{cfg: config.Config{}, llm: fake}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"我很难过"}`))
	h.ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, "抱抱你", "gRPC 失败必须回落 mock 话术")
	assert.Contains(t, body, "data: [DONE]")
}
