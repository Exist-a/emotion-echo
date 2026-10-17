// Package handler — tts_handler_voice_test.go
//
// CL-06 · Phase 6：`/api/v1/tts/phonemes` 的 `voice` 字段**必须原样传到下游 provider**。
//
// 为什么单独钉这一条：本仓反复出现"请求结构里没有该字段 ⇒ 静默丢弃"的缺陷
// （E2E-F-12 的 `shouldGenerateTitle`、Stage 79 的 `contentType` 三个 tag 都是这个形态）。
// 加了 provider 层的覆盖逻辑还不够 —— 只要 handler 的请求结构漏了该字段，前端发什么都白搭。
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPhonemesRouter(p downstream.TTSProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewTTSHandler(nil, nil, p).Register(r)
	return r
}

func TestTTSHandler_Phonemes_PassesVoiceToProvider(t *testing.T) {
	p := &fakeTTSProvider{resp: &downstream.XTTSPhonemesResp{
		Audio: "AAA", Phonemes: []downstream.XTTSPhoneme{{Char: "你"}}, Duration: 0.2, Text: "你好",
	}}
	r := newPhonemesRouter(p)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes",
		strings.NewReader(`{"text":"你好","voice":"FunAudioLLM/CosyVoice2-0.5B:alex"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, p.lastReq)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:alex", p.lastReq.Voice,
		"handler 必须把 voice 原样交给 provider（否则前端传了也白搭）")
	assert.Equal(t, "你好", p.lastReq.Text)
	assert.Contains(t, w.Body.String(), `"phonemes"`)
}

// 零回归：不带 voice ⇒ 不注入任何值（provider 侧据此回落配置默认）
func TestTTSHandler_Phonemes_NoVoice_LeavesEmpty(t *testing.T) {
	p := &fakeTTSProvider{resp: &downstream.XTTSPhonemesResp{Audio: "AAA", Duration: 0.2}}
	r := newPhonemesRouter(p)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes", strings.NewReader(`{"text":"你好"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, p.lastReq)
	assert.Empty(t, p.lastReq.Voice, "未携带 voice ⇒ 保持空（provider 回落配置默认）")
}

// 响应结构不受影响（口型链路的三个字段仍在）—— 防"改了请求顺手把响应也改了"
func TestTTSHandler_Phonemes_ResponseStillHasDurationAndAudio(t *testing.T) {
	resp := &downstream.XTTSPhonemesResp{
		Audio:     "QUJD",
		Phonemes:  []downstream.XTTSPhoneme{{Char: "你", Start: 0, Duration: 0.1}},
		Duration:  0.1,
		Text:      "你",
		SampleRate: 24000,
	}
	p := &fakeTTSProvider{resp: resp}
	r := newPhonemesRouter(p)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes",
		strings.NewReader(`{"text":"你","voice":"FunAudioLLM/CosyVoice2-0.5B:bella"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "响应必须是 {code,message,data} 信封")
	assert.Contains(t, data, "audio")
	assert.Contains(t, data, "phonemes")
	assert.Contains(t, data, "duration")
	assert.Equal(t, float64(24000), data["sample_rate"], "响应字段名沿用 XTTS 契约的 snake_case（前端已按此消费）")
}

// 未装配 provider ⇒ 503（照旧，本阶段不动这条语义）
func TestTTSHandler_Phonemes_NoProvider_503(t *testing.T) {
	r := newPhonemesRouter(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes", strings.NewReader(`{"text":"你好"}`))
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
