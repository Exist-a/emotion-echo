// Package handler — ai_stream_personality_test.go
//
// E2E-14：人格画像注入 AI system prompt。
//
// 契约（D-02）：
//  1. 用户有人格量表结果 → system prompt 追加可读画像文本（五维度 + 高/中/低）
//  2. 用户无人格量表结果 → 回落基础人设 prompt（绝不因此失败）
//  3. 画像来源报错 → 同上回落 + 记日志（画像注入是增强，不是依赖）
//
// 基底人设 prompt 在两处上游路径（gRPC / HTTP 直连）共用，注入点必须同一。
package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePersonalitySource 实现 personalitySource
type fakePersonalitySource struct {
	dims  map[string]float64
	err   error
	calls int
}

func (f *fakePersonalitySource) LatestPersonalityProfile(context.Context) (map[string]float64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.dims, nil
}

func newPersonalityRouter(streamer *fakeLLMStreamer, src personalitySource) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &AIStreamHandler{cfg: config.Config{}, llm: streamer, personality: src}
	r.POST("/api/v1/ai/stream", h.ServeHTTP)
	return r
}

// TestAIStreamHandler_Personality_InjectedIntoSystemPrompt 有人格结果 → 注入画像适配指令
//
// 注：注入内容的**具体措辞契约**已迁到 personality_directive_test.go
// （语义化后不再是「形容词罗列」，而是逐条行为指令）。本用例只守
// 「有画像 ⇒ 进 system prompt」这条通路 + 来源被查询一次。
func TestAIStreamHandler_Personality_InjectedIntoSystemPrompt(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"hi"}}
	src := &fakePersonalitySource{dims: map[string]float64{
		"openness":          26,
		"conscientiousness": 18,
		"extraversion":      24,
		"agreeableness":     19,
		"neuroticism":       10,
	}}
	router := newPersonalityRouter(fake, src)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"今天有点累","conversationId":"1"}`))
	router.ServeHTTP(w, req)

	require.NotEmpty(t, fake.gotMsgs)
	require.Equal(t, "system", fake.gotMsgs[0].Role)
	sys := fake.gotMsgs[0].Content
	// 基础人设仍在，且是前缀（基底不被画像改写）
	assert.True(t, strings.HasPrefix(sys, baseSystemPrompt), "基础人设必须原样保留在开头")
	// 适配层进入
	assert.Contains(t, sys, "与这位用户相处的方式")
	// 五维数字摘要齐全（含中档维度）
	for _, label := range []string{"开放性", "尽责性", "外向性", "宜人性", "神经质"} {
		assert.Contains(t, sys, label, "system prompt 必须含维度标签 %s", label)
	}
	assert.Contains(t, sys, "26/30", "应带五维数字摘要")
	assert.Equal(t, 1, src.calls, "画像来源每次请求查询一次")
}

// TestAIStreamHandler_NoPersonality_UsesBasePrompt 无人格结果 → 纯基础人设，不失败
func TestAIStreamHandler_NoPersonality_UsesBasePrompt(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"hi"}}
	src := &fakePersonalitySource{dims: nil}
	router := newPersonalityRouter(fake, src)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"你好"}`))
	router.ServeHTTP(w, req)

	require.NotEmpty(t, fake.gotMsgs)
	sys := fake.gotMsgs[0].Content
	assert.Equal(t, baseSystemPrompt, sys, "无结果时必须与无画像时逐字相同（否则是编造画像）")
	assert.Contains(t, w.Body.String(), "hi", "无画像也必须正常回复")
}

// TestAIStreamHandler_PersonalitySourceError_FallsBackToBasePrompt 来源报错 → 回落，不阻断对话
func TestAIStreamHandler_PersonalitySourceError_FallsBackToBasePrompt(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"hi"}}
	src := &fakePersonalitySource{err: assert.AnError}
	router := newPersonalityRouter(fake, src)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"你好"}`))
	router.ServeHTTP(w, req)

	require.NotEmpty(t, fake.gotMsgs)
	assert.Equal(t, baseSystemPrompt, fake.gotMsgs[0].Content)
	assert.Contains(t, w.Body.String(), "hi")
}

// TestAIStreamHandler_NilPersonalitySource_UsesBasePrompt 未装配来源（nil）→ 基础人设
func TestAIStreamHandler_NilPersonalitySource_UsesBasePrompt(t *testing.T) {
	fake := &fakeLLMStreamer{deltas: []string{"hi"}}
	router := newPersonalityRouter(fake, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ai/stream",
		strings.NewReader(`{"message":"你好"}`))
	router.ServeHTTP(w, req)

	require.NotEmpty(t, fake.gotMsgs)
	assert.Contains(t, fake.gotMsgs[0].Content, "情绪疏导陪伴者")
	assert.NotContains(t, fake.gotMsgs[0].Content, "与这位用户相处的方式")
}

// 注：原 `formatPersonalityContext` 的三个单测已随该函数一并删除 ——
// 它产出的是「形容词罗列」（开放性高（30/30）、…），即账本 E2E-F-95 认定语义不足的那版。
// 替代实现在 personality_directive.go，契约测试见 personality_directive_test.go。
