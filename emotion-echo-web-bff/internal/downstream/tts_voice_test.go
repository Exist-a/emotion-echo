// Package downstream — tts_voice_test.go
//
// CL-06 · Phase 6：**每请求音色**（T-12/T-13）。
//
// 现状（CL-06 前）：音色是**构造期全局单值** —— `etc/web-bff.yaml` 的
// `Voice: FunAudioLLM/CosyVoice2-0.5B:anna` 被 provider 读一次，只有维护一个音色。
// CL-06 要求"音色随角色变" ⇒ 请求体新增可选 `voice`，由 cloud provider **覆盖**配置默认值。
//
// 零回归的判据（关键）：**不带该字段 ⇒ 上游收到的 voice 与现状逐字相同**。
// 因此本文件的正例必须成对出现：带字段（覆盖）与不带字段（回落）。
//
// ⚠️ 已知边界（阶段档 T-18 如实登记）：降级到 XTTS 时没有这 8 个音色 ——
// local provider 会把 req 原样转给 XTTS（多余字段被上游忽略），音色在降级窗口内**回落**。
// 该窗口不被本阶段的音色验收覆盖（主链路是 cloud，见计划 §A.5）。
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureSpeechServer 起一个只记录上游请求体的假 SiliconFlow。
func captureSpeechServer(t *testing.T) (*httptest.Server, *siliconFlowSpeechReq, *[]byte) {
	t.Helper()
	var got siliconFlowSpeechReq
	var raw []byte
	wav := buildWav(t, 24000, 1, 16, 2400) // 0.1s
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &raw
}

// T-12 正例：带 voice ⇒ 上游 voice = 请求值（覆盖配置默认）
func TestCloudProvider_RequestVoice_OverridesConfigDefault(t *testing.T) {
	srv, got, _ := captureSpeechServer(t)
	p := NewCloudTTSProvider(cloudTestOpts(srv.URL)) // 配置默认 = :anna

	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{
		Text: "你好", Voice: "FunAudioLLM/CosyVoice2-0.5B:alex",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:alex", got.Voice,
		"请求携带的音色必须覆盖配置默认值（T-12）")
}

// T-12 零回归：不带 voice ⇒ 上游 voice 仍是配置默认（现状逐字不变）
func TestCloudProvider_NoRequestVoice_FallsBackToConfigDefault(t *testing.T) {
	srv, got, _ := captureSpeechServer(t)
	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))

	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "你好"})
	require.NoError(t, err)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:anna", got.Voice,
		"不带 voice 时必须回落配置默认（零回归）")
}

// 纯空白同样按"未提供"处理（否则一个空格就能把音色打成空串，上游 400）
func TestCloudProvider_WhitespaceVoice_FallsBackToConfigDefault(t *testing.T) {
	srv, got, _ := captureSpeechServer(t)
	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))

	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "你好", Voice: "   "})
	require.NoError(t, err)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:anna", got.Voice)
}

// 请求体里的中文与音色字段共存时不互相干扰（UTF-8 原文 + voice 原样）
func TestCloudProvider_VoiceAndChineseText_BothIntact(t *testing.T) {
	srv, got, raw := captureSpeechServer(t)
	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))

	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{
		Text: "我今天有点累", Voice: "FunAudioLLM/CosyVoice2-0.5B:bella",
	})
	require.NoError(t, err)
	assert.Equal(t, "我今天有点累", got.Input)
	assert.True(t, bytes.Contains(*raw, []byte("我今天有点累")), "中文必须 UTF-8 原文")
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:bella", got.Voice)
}

// T-13 口型不回归：带 voice 的请求仍返回 phonemes 数组 + duration（G4 硬约束）
func TestCloudProvider_WithVoice_StillReturnsPhonemesAndDuration(t *testing.T) {
	srv, _, _ := captureSpeechServer(t)
	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))

	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{
		Text: "你好世界", Voice: "FunAudioLLM/CosyVoice2-0.5B:claire",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "你好世界", resp.Text)
	assert.NotEmpty(t, resp.Audio, "audio 必须回传（前端三项联合硬校验之一）")
	assert.NotEmpty(t, resp.Phonemes, "phonemes 必须回传（口型链硬约束）")
	assert.Len(t, resp.Phonemes, 4, "per-char 等分：4 个字 ⇒ 4 条")
	assert.Greater(t, resp.Duration, 0.0, "duration 必须 > 0（前端硬校验之一）")
}

// 请求体契约：voice 字段名与前端一致（frontend 发 voice，不是 voiceId / speaker）
func TestTTSPhonemesReq_VoiceJSONContract(t *testing.T) {
	var req TTSPhonemesReq
	require.NoError(t, json.Unmarshal([]byte(`{"text":"你好","voice":"FunAudioLLM/CosyVoice2-0.5B:alex"}`), &req))
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:alex", req.Voice, "字段名必须是 voice")

	// 不带 voice 时不得在 JSON 里冒出一个空 voice（避免下游把空串当有效值）
	raw, err := json.Marshal(TTSPhonemesReq{Text: "你好"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "voice", "空 voice 必须 omitempty（零回归：请求体与现状一致）")
}

// local（XTTS）回退：req 原样转给 XTTS —— 音色在降级窗口内回落，属**已知边界**
// （如实登记，不在本阶段验收范围内；见文件头注释与计划 §A.5/T-18）
func TestLocalProvider_PassesRequestThrough_IgnoringVoice(t *testing.T) {
	fake := &recordingXTTS{}
	p := NewLocalTTSProvider(fake)

	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{
		Text: "你好", Voice: "FunAudioLLM/CosyVoice2-0.5B:alex", Speed: 0.75,
	})
	require.NoError(t, err)
	require.NotNil(t, fake.got)
	assert.Equal(t, "你好", fake.got.Text)
	assert.Equal(t, 0.75, fake.got.Speed)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:alex", fake.got.Voice,
		"voice 会原样转给 XTTS（上游无此概念、忽略之）—— 降级窗口音色回落是已知边界")
}

// recordingXTTS 记录收到的请求（XTTSClient 的最小测试替身）
type recordingXTTS struct {
	got *TTSPhonemesReq
}

func (f *recordingXTTS) Phonemes(_ context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	f.got = &req
	return &XTTSPhonemesResp{Text: req.Text}, nil
}

func (f *recordingXTTS) Stream(_ context.Context, _ TTSStreamReq) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (f *recordingXTTS) Health(_ context.Context) (*XTTSHealthResp, error) {
	return &XTTSHealthResp{Status: "ok"}, nil
}
