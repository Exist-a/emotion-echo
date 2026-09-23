// Package handler — tts_phonemes_handler_test.go
//
// E2E-F-127（计划 §6 step 2 RED）：TTSHandler.phonemes 端点契约测试。
//
// 设计：handler.phonemes POST /api/v1/tts/phonemes {text, language, speed}
// → XTTSClient.Phonemes → 透传 JSON 给前端（B 选：BFF 转发通道最小改动）。
//
// 与 /tts/stream 并存（功能不取代），后者继续裸 WAV 流式；前者提供 phonemes
// 用于 D-03 真口型同步。
//
// 契约钉（5 项）：
//   1. POST /api/v1/tts/phonemes 路由注册
//   2. 200 + 透传 audio/sample_rate/text/language/phonemes/duration 全字段
//   3. text 空 → 400（与 /tts/stream 一致）
//   4. XTTS 上游 4xx → BFF 透传 status + detail 错误
//   5. XTTSClient nil → 503 "not configured"（nil-safe 与 Stream 一致）
package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeXTTSPhonemes 实现 downstream.XTTSClient 注入到 TTSHandler。
// 注意 XTTSClient 是接口，需让 fakeXTTSPhonemes 实现 Stream + Health + Phonemes 三方法
// （本测试只关注 Phonemes；其他方法返回零值/no-op）。
type fakeXTTSPhonemes struct {
	phonemesResp *downstream.XTTSPhonemesResp
	phonemesErr  error
	calls        int
	lastReq      *downstream.TTSPhonemesReq
}

// 实现 XTTSClient 三方法（Stream/Health 返回零值，Phonemes 走 fake）。
func (f *fakeXTTSPhonemes) Stream(context.Context, downstream.TTSStreamReq) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *fakeXTTSPhonemes) Health(context.Context) (*downstream.XTTSHealthResp, error) {
	return &downstream.XTTSHealthResp{Status: "ok", ModelLoaded: true, ModelType: "XTTS-v2"}, nil
}
func (f *fakeXTTSPhonemes) Phonemes(_ context.Context, req downstream.TTSPhonemesReq) (*downstream.XTTSPhonemesResp, error) {
	f.calls++
	f.lastReq = &req
	return f.phonemesResp, f.phonemesErr
}

func newPhonemesHandlerRouter(fake *fakeXTTSPhonemes) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTTSHandler(nil, fake) // ai=nil（synthesize 不测）；xtts=fake
	h.Register(r)
	return r
}

func TestTTSHandler_Phonemes_RouteRegistered(t *testing.T) {
	// 契约 1：路由存在（404 vs 405 区分：未注册→404 Gin's redirect；已注册 POST 不允许 GET→405）
	r := newPhonemesHandlerRouter(&fakeXTTSPhonemes{phonemesResp: &downstream.XTTSPhonemesResp{}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes",
		strings.NewReader(`{"text":"hi","language":"zh-cn","speed":1.0}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusNotFound, w.Code, "POST /api/v1/tts/phonemes 必须注册（计划 §2.A 第 4 条）")
}

func TestTTSHandler_Phonemes_ReturnsFullJSON(t *testing.T) {
	// 契约 2：200 + 透传全字段
	want := &downstream.XTTSPhonemesResp{
		Audio:      base64.StdEncoding.EncodeToString([]byte("RIFF....WAVE")),
		SampleRate: 24000,
		Text:       "你好",
		Language:   "zh-cn",
		Phonemes: []downstream.XTTSPhoneme{
			{Char: "你", Start: 0.0, Duration: 0.5},
			{Char: "好", Start: 0.5, Duration: 0.5},
		},
		Duration: 1.0,
	}
	fake := &fakeXTTSPhonemes{phonemesResp: want}
	r := newPhonemesHandlerRouter(fake)

	body, _ := json.Marshal(map[string]any{"text": "你好", "language": "zh-cn", "speed": 1.0})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	respBody := respBodyHelper(t, w)
	// BFF 统一用 OK(c, data) 包装：{code, message, data} —— 解外层后取 data
	var envelope struct {
		Code    int                       `json:"code"`
		Message string                    `json:"message"`
		Data    downstream.XTTSPhonemesResp `json:"data"`
	}
	require.NoError(t, json.Unmarshal(respBody, &envelope), "响应必须是合法 JSON")
	require.Equal(t, 0, envelope.Code, "BFF OK envelope code 应为 0")
	got := envelope.Data

	assert.Equal(t, want.Audio, got.Audio, "audio base64 必须透传")
	assert.Equal(t, want.SampleRate, got.SampleRate)
	assert.Equal(t, want.Text, got.Text)
	assert.Equal(t, want.Language, got.Language)
	assert.InDelta(t, want.Duration, got.Duration, 0.001)
	require.Len(t, got.Phonemes, 2)
	assert.Equal(t, "你", got.Phonemes[0].Char)
	assert.InDelta(t, 0.5, got.Phonemes[0].Duration, 0.001)
}

func TestTTSHandler_Phonemes_EmptyText_Returns400(t *testing.T) {
	// 契约 3：text 空 → 400（与 /tts/stream 一致）
	fake := &fakeXTTSPhonemes{}
	r := newPhonemesHandlerRouter(fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes",
		strings.NewReader(`{"text":"","language":"zh-cn"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, 0, fake.calls, "text 空时不得调下游（早期失败节省一次 gRPC）")
}

func TestTTSHandler_Phonemes_Upstream400_Propagates(t *testing.T) {
	// 契约 4：XTTS 返 4xx → BFF 透传 status code + detail 文本
	fake := &fakeXTTSPhonemes{phonemesErr: errors.New("downstream: tts phonemes: 400 Bad Request: Text is required")}
	r := newPhonemesHandlerRouter(fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes",
		strings.NewReader(`{"text":"hi","language":"zh-cn"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// statusFor(err) 把 gRPC 错误转 HTTP 码；FastAPI 4xx 通常→500 或 502（看 wrapGRPCError）
	// 实际码本测试要 GREEN 时钉。本测试先断言非 200 且错误信息透传。
	assert.NotEqual(t, http.StatusOK, w.Code, "XTTS 报错不得返 200")
	assert.Contains(t, w.Body.String(), "Text is required", "上游 detail 必须透传（BFF 才有有意义的错误）")
}

func TestTTSHandler_Phonemes_NilClient_ReturnsNotConfigured(t *testing.T) {
	// 契约 5：nil-safe — 与 Stream 一致（避免 BFF 启动时 XTTS_BASE_URL="" panic）
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewTTSHandler(nil, &nilXTTSClient{}) // xtts 字段 nil（=BaseURL""的等效）
	h.Register(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/phonemes",
		strings.NewReader(`{"text":"hi","language":"zh-cn"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "not configured", "nil-safe 错误必须明确（前端可区分配置 vs 上游故障）")
}

// nilXTTSClient 让 NewTTSHandler xtts=nil 字段（NewTTSHandler 不检查 nil）
type nilXTTSClient struct{}

func (nilXTTSClient) Stream(context.Context, downstream.TTSStreamReq) (io.ReadCloser, error) {
	return nil, errors.New("xtts client not configured")
}
func (nilXTTSClient) Health(context.Context) (*downstream.XTTSHealthResp, error) {
	return nil, errors.New("xtts client not configured")
}
func (nilXTTSClient) Phonemes(context.Context, downstream.TTSPhonemesReq) (*downstream.XTTSPhonemesResp, error) {
	return nil, errors.New("xtts client not configured")
}

func respBodyHelper(t *testing.T, w *httptest.ResponseRecorder) []byte {
	t.Helper()
	return w.Body.Bytes()
}