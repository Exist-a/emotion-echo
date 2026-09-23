// Package downstream — xtts_phonemes_test.go
//
// E2E-F-127（计划 §6 step 2 RED）：XTTSClient.Phonemes 契约测试。
//
// 背景：D-03 真口型同步需要从 XTTS 取字符级时间戳（per-char 等分近似）。
// vendor app.py 缺端点已修（F-126 关闭，仓镜像 emotion-echo/xtts:v2.0.0 已上），
// 现在补齐 BFF → XTTS 的 Phonemes 通道（A 选 —— 改动最小 + 不破坏既有消费者）。
//
// 设计：XTTSClient 新增 Phonemes(ctx, req) → *PhonemesResp, error；直连 XTTS
// POST /tts_with_phonemes，复用现有 xttsHTTPClient.BaseURL/TimeoutMs（与 Stream 同源）。
//
// 字段契约（仓 server.py:319-326 + 实测 #3/#4）：
//   - audio: base64-encoded WAV bytes（与 /tts_stream 同采样率 24000）
//   - sample_rate, text, language, duration: 透传
//   - phonemes: [{char, start_秒, duration_秒}] per-char 等分
//
// 测试隔离：与 xtts_test.go 同样的 httptest.NewServer + fake XTTS 模式。
package downstream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 复用 xtts_test.go 的 fakeXTTSForTTS 模式（最小化：仅 path/method 断言 + 响应）
// 此处另写一版以便 phonemes path 独立演进。

func TestXTTSClient_Phonemes_PostsCorrectPath(t *testing.T) {
	var seenPath, seenMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenMethod = r.Method
		_ = json.NewEncoder(w).Encode(XTTSPhonemesResp{
			Audio:      base64.StdEncoding.EncodeToString([]byte("RIFF....WAVE")),
			SampleRate: 24000,
			Text:       "hello",
			Language:   "zh-cn",
			Phonemes:   []XTTSPhoneme{{Char: "h", Start: 0.0, Duration: 0.5}},
			Duration:   0.5,
		})
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	resp, err := c.Phonemes(context.Background(), TTSPhonemesReq{Text: "hello", Language: "zh-cn", Speed: 1.0})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "/tts_with_phonemes", seenPath, "必须调 /tts_with_phonemes 端点（F-126 落地后这是唯一路径）")
	assert.Equal(t, http.MethodPost, seenMethod)
}

func TestXTTSClient_Phonemes_PassesRequestFields(t *testing.T) {
	var gotReq TTSPhonemesReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotReq))
		_ = json.NewEncoder(w).Encode(XTTSPhonemesResp{
			Phonemes: []XTTSPhoneme{},
			Duration: 0,
		})
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.Phonemes(context.Background(), TTSPhonemesReq{
		Text:     "你好世界",
		Language: "zh-cn",
		Speed:    1.0,
	})
	require.NoError(t, err)
	assert.Equal(t, "你好世界", gotReq.Text)
	assert.Equal(t, "zh-cn", gotReq.Language)
	assert.InDelta(t, 1.0, gotReq.Speed, 0.001)
}

func TestXTTSClient_Phonemes_DecodesAllFields(t *testing.T) {
	wantAudio := base64.StdEncoding.EncodeToString([]byte("RIFF....WAVEfmt data"))
	wantPhonemes := []XTTSPhoneme{
		{Char: "你", Start: 0.0, Duration: 0.323},
		{Char: "好", Start: 0.323, Duration: 0.323},
		{Char: "界", Start: 0.968, Duration: 0.323},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(XTTSPhonemesResp{
			Audio:      wantAudio,
			SampleRate: 24000,
			Text:       "你好世界",
			Language:   "zh-cn",
			Phonemes:   wantPhonemes,
			Duration:   1.291,
		})
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	resp, err := c.Phonemes(context.Background(), TTSPhonemesReq{Text: "你好世界"})
	require.NoError(t, err)
	require.NotNil(t, resp)

	decodedAudio, err := base64.StdEncoding.DecodeString(resp.Audio)
	require.NoError(t, err)
	assert.Equal(t, []byte("RIFF....WAVEfmt data"), decodedAudio, "audio 必须是 base64-encoded WAV（前端拿到要能 decode）")
	assert.Equal(t, 24000, resp.SampleRate)
	assert.Equal(t, "你好世界", resp.Text)
	assert.Equal(t, "zh-cn", resp.Language)
	assert.InDelta(t, 1.291, resp.Duration, 0.001)
	require.Len(t, resp.Phonemes, 3)
	assert.Equal(t, "你", resp.Phonemes[0].Char)
	assert.InDelta(t, 0.323, resp.Phonemes[0].Duration, 0.001)
}

func TestXTTSClient_Phonemes_Upstream400_ReturnsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Text is required"})
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.Phonemes(context.Background(), TTSPhonemesReq{Text: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Text is required", "FastAPI detail 必须提取（BFF 才有有意义错误透传给前端）")
}

func TestXTTSClient_Phonemes_NilClientOrEmptyBaseURL_ReturnsError(t *testing.T) {
	// nil-safe 契约：与 Stream 同款（避免 BFF 启动时 XTTS_BASE_URL="" 崩溃）
	var nilSrc *xttsHTTPClient
	_, err := nilSrc.Phonemes(context.Background(), TTSPhonemesReq{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")

	c := NewXTTSClient(XTTSClientOptions{BaseURL: "", TimeoutMs: 1000})
	_, err = c.Phonemes(context.Background(), TTSPhonemesReq{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}