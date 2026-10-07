// Package handler — tts_handler_provider_test.go
//
// E2E-F-198（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D T7）RED：
// TTSHandler.phonemes 改经 TTSProvider（cloud=CosyVoice2 / local=XTTS 回退），
// /tts/stream 保持 XTTS 直连不动（M2-A：tier-2 流式另立项）。
//
// 构造函数扩为 NewTTSHandler(ai, xtts, tts)——xtts 仍供 /tts/stream 使用；
// phonemes 的上游选择/回退语义全部收敛在 provider（downstream 层）。
// nil provider → 503 "tts provider not configured"（与 XTTSClient nil-safe 同纪律）。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTTSProvider 可编程假 provider（记录调用）。
type fakeTTSProvider struct {
	resp    *downstream.XTTSPhonemesResp
	err     error
	calls   int
	lastReq *downstream.TTSPhonemesReq
}

func (f *fakeTTSProvider) Synthesize(_ context.Context, req downstream.TTSPhonemesReq) (*downstream.XTTSPhonemesResp, error) {
	f.calls++
	f.lastReq = &req
	return f.resp, f.err
}
func (f *fakeTTSProvider) Name() string { return "fake" }

func newProviderHandlerRouter(p downstream.TTSProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTTSHandler(nil, nilXTTSClient{}, p) // ai 不测；xtts 给非 nil fake（stream 路径无 nil-interface 守卫，与既有行为一致）
	h.Register(r)
	return r
}

func postPhonemes(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTTSHandler_Phonemes_UsesProvider(t *testing.T) {
	p := &fakeTTSProvider{resp: &downstream.XTTSPhonemesResp{
		Audio:      "YXVkaW8=",
		SampleRate: 24000,
		Text:       "你好",
		Language:   "zh-cn",
		Phonemes:   []downstream.XTTSPhoneme{{Char: "你", Start: 0, Duration: 0.3}},
		Duration:   0.6,
	}}
	r := newProviderHandlerRouter(p)

	w := postPhonemes(t, r, `{"text":"你好","language":"zh-cn","speed":0.75,"volume":2.0}`)
	require.Equal(t, http.StatusOK, w.Code)

	assert.EqualValues(t, 1, p.calls, "phonemes 必须经 provider（cloud/local 回退语义在 downstream 层收敛）")
	require.NotNil(t, p.lastReq)
	assert.Equal(t, "你好", p.lastReq.Text, "请求体必须透传给 provider")

	var env struct {
		Code int                         `json:"code"`
		Data downstream.XTTSPhonemesResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, 0, env.Code, "OK 信封 code=0")
	assert.Equal(t, "YXVkaW8=", env.Data.Audio)
	assert.InDelta(t, 0.6, env.Data.Duration, 0.001)
	require.Len(t, env.Data.Phonemes, 1)
}

func TestTTSHandler_Phonemes_ProviderError_MapsStatus(t *testing.T) {
	p := &fakeTTSProvider{err: assert2Error("downstream: tts cloud: status 401: Invalid API key")}
	r := newProviderHandlerRouter(p)

	w := postPhonemes(t, r, `{"text":"你好"}`)
	assert.GreaterOrEqual(t, w.Code, 400, "provider 错误必须映射为 4xx/5xx（statusFor 既有语义）")

	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.NotEqual(t, 0, env.Code, "Fail 信封 code!=0")
	assert.Contains(t, env.Message, "Invalid API key", "上游错误 detail 必须透传（前端可排查）")
}

func TestTTSHandler_Phonemes_NilProvider_Returns503(t *testing.T) {
	r := newProviderHandlerRouter(nil)

	w := postPhonemes(t, r, `{"text":"你好"}`)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "nil provider = 装配缺失，503（与 XTTSClient nil-safe 同纪律）")
}

func TestTTSHandler_Stream_UnaffectedByProvider(t *testing.T) {
	// /tts/stream 仍走 xtts（M2-A：cloud 流式另立项）——xtts=nil 时应报
	// "not configured" 而不是触碰 provider。
	p := &fakeTTSProvider{resp: &downstream.XTTSPhonemesResp{}}
	r := newProviderHandlerRouter(p)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/stream", bytes.NewBufferString(`{"text":"你好"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.EqualValues(t, 0, p.calls, "/tts/stream 不得触碰 provider")
	assert.GreaterOrEqual(t, w.Code, 400)
}

// assert2Error 构造带文本的 error（避免引入 errors.New 之外的依赖噪音）。
func assert2Error(msg string) error { return &staticError{msg} }

type staticError struct{ msg string }

func (e *staticError) Error() string { return e.msg }
