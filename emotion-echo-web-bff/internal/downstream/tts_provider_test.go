// Package downstream — tts_provider_test.go
//
// E2E-F-198（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D T1）RED：
// TTSProvider 统一接口 + localProvider 包装既有 XTTSClient。
//
// 设计（plan §C）：
//   - TTSProvider.Synthesize 复用 XTTSPhonemesResp 作为统一返回类型 ⇒ 前端契约
//     （useTTSPlayer.ts:215 三项硬校验）与 handler OK() 信封零改动
//   - localProvider 纯转调 XTTSClient.Phonemes（零行为变更，回退路径）
//   - Name() 供日志/指标区分 "cloud" | "local"
//
// 测试隔离：与 xtts_phonemes_test.go 同款 httptest.NewServer + fake XTTS 模式。
package downstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalProvider_Synthesize_DelegatesToXTTS(t *testing.T) {
	var seenPath, seenMethod string
	var gotReq TTSPhonemesReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenMethod = r.Method
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotReq))
		_ = json.NewEncoder(w).Encode(XTTSPhonemesResp{
			Audio:      base64.StdEncoding.EncodeToString([]byte("RIFF....WAVE")),
			SampleRate: 24000,
			Text:       "你好世界",
			Language:   "zh-cn",
			Phonemes: []XTTSPhoneme{
				{Char: "你", Start: 0, Duration: 0.25},
				{Char: "好", Start: 0.25, Duration: 0.25},
				{Char: "世", Start: 0.5, Duration: 0.25},
				{Char: "界", Start: 0.75, Duration: 0.25},
			},
			Duration: 1.0,
		})
	}))
	defer srv.Close()

	xtts := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	p := NewLocalTTSProvider(xtts)
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "你好世界", Language: "zh-cn", Speed: 0.75})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "/tts_with_phonemes", seenPath, "localProvider 必须转调 XTTS /tts_with_phonemes（回退路径零行为变更）")
	assert.Equal(t, http.MethodPost, seenMethod)
	assert.Equal(t, "你好世界", gotReq.Text, "请求体必须原样透传")
	assert.InDelta(t, 0.75, gotReq.Speed, 0.001)

	// 响应透传（前端硬校验的三件套：audio/phonemes/duration 一个不能少）
	assert.NotEmpty(t, resp.Audio)
	assert.InDelta(t, 1.0, resp.Duration, 0.001)
	require.Len(t, resp.Phonemes, 4)
	assert.Equal(t, "你", resp.Phonemes[0].Char)
}

func TestLocalProvider_Name_IsLocal(t *testing.T) {
	p := NewLocalTTSProvider(nil)
	assert.Equal(t, "local", p.Name(), "Name() 供降级日志与指标 label 消费")
}

func TestLocalProvider_WrapsXTTSError(t *testing.T) {
	// XTTS 未配置（BaseURL 空）→ 错误必须原样上抛，不得被 provider 吞掉改写
	xtts := NewXTTSClient(XTTSClientOptions{BaseURL: "", TimeoutMs: 1000})
	p := NewLocalTTSProvider(xtts)
	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured", "错误透传契约与 XTTSClient 一致")
}

// ===== T2：cloud provider（SiliconFlow /v1/audio/speech）请求契约 =====
//
// 契约来源：SiliconFlow 官方文档（plan §B.4）+ ADR-2026-10 实测：
//   - POST {BaseURL}/audio/speech，Authorization: Bearer <key>
//   - body: model / input / voice / response_format / sample_rate / speed / gain
//   - response_format 必须 wav（前端 base64ToWavBlob 不构造 WAV 头，plan §B.3）
//   - sample_rate 必须显式 24000（官方默认 44100，plan §B.4 / H4）
//   - 中文 payload 必须 UTF-8 原文（ADR 实施注意：编码损坏 = 合成"流利但胡说"）

// siliconFlowSpeechReq 是 fake 服务端看到的请求体形状（与官方文档字段名一一对应）。
type siliconFlowSpeechReq struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format"`
	SampleRate     int      `json:"sample_rate"`
	Speed          float64  `json:"speed"`
	Gain           *float64 `json:"gain"`
	Stream         bool     `json:"stream"`
}

// buildWav 构造已知尺寸的极小 WAV（RIFF/WAVE + fmt + data），供各测试复用。
func buildWav(t *testing.T, sampleRate, channels, bitsPerSample, numSamples int) []byte {
	t.Helper()
	dataBytes := numSamples * channels * (bitsPerSample / 8)
	byteRate := sampleRate * channels * (bitsPerSample / 8)
	blockAlign := channels * (bitsPerSample / 8)

	wav := make([]byte, 0, 44+dataBytes)
	appendStr := func(s string) { wav = append(wav, s...) }
	appendU32 := func(v uint32) {
		wav = append(wav, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	appendU16 := func(v uint16) {
		wav = append(wav, byte(v), byte(v>>8))
	}
	appendStr("RIFF")
	appendU32(uint32(36 + dataBytes))
	appendStr("WAVE")
	appendStr("fmt ")
	appendU32(16)
	appendU16(1) // PCM
	appendU16(uint16(channels))
	appendU32(uint32(sampleRate))
	appendU32(uint32(byteRate))
	appendU16(uint16(blockAlign))
	appendU16(uint16(bitsPerSample))
	appendStr("data")
	appendU32(uint32(dataBytes))
	wav = append(wav, make([]byte, dataBytes)...)
	return wav
}

func cloudTestOpts(baseURL string) CloudTTSOptions {
	return CloudTTSOptions{
		BaseURL:    baseURL,
		APIKey:     "test-key",
		Model:      "FunAudioLLM/CosyVoice2-0.5B",
		Voice:      "FunAudioLLM/CosyVoice2-0.5B:anna",
		SampleRate: 24000,
		TimeoutMs:  1000,
	}
}

func TestCloudProvider_PostsCorrectContract(t *testing.T) {
	var seenPath, seenAuth, seenContentType string
	var gotBody siliconFlowSpeechReq
	var rawBody []byte
	wav := buildWav(t, 24000, 1, 16, 24000) // 0.5s @24kHz mono int16
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		seenContentType = r.Header.Get("Content-Type")
		rawBody, _ = io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(rawBody, &gotBody))
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	defer srv.Close()

	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{
		Text: "你好世界", Language: "zh-cn", Speed: 0.75, Volume: 2.0,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "/audio/speech", seenPath, "BaseURL 已含 /v1，provider 只补 /audio/speech")
	assert.Equal(t, "Bearer test-key", seenAuth)
	assert.Contains(t, seenContentType, "application/json")

	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B", gotBody.Model)
	assert.Equal(t, "你好世界", gotBody.Input)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:anna", gotBody.Voice)
	assert.Equal(t, "wav", gotBody.ResponseFormat, "必须 wav 容器（前端 base64ToWavBlob 不建头，plan §B.3）")
	assert.Equal(t, 24000, gotBody.SampleRate, "必须显式 pin 24000（官方默认 44100，plan §B.4）")

	// 中文 payload UTF-8 原文（ADR 实施注意：禁 mojibake）
	assert.True(t, bytes.Contains(rawBody, []byte("你好世界")),
		"请求体必须是 UTF-8 中文原文（非转义/mojibake）")

	// 音频原样 base64 回传（T3 才补 phonemes/duration）
	decoded, err := base64.StdEncoding.DecodeString(resp.Audio)
	require.NoError(t, err)
	assert.Equal(t, wav, decoded)
	assert.Equal(t, 24000, resp.SampleRate)
	assert.Equal(t, "你好世界", resp.Text)
	assert.Equal(t, "zh-cn", resp.Language)
}

func TestCloudProvider_SpeedClamped(t *testing.T) {
	// XTTS 默认 speed=0.75；SiliconFlow 合法域 [0.25, 4.0]，默认 1.0
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"unset uses upstream default 1.0", 0, 1.0},
		{"xtts default 0.75 passes through", 0.75, 0.75},
		{"over range clamps to 4.0", 10, 4.0},
		{"under range clamps to 0.25", 0.1, 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody siliconFlowSpeechReq
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				_, _ = w.Write(buildWav(t, 24000, 1, 16, 240))
			}))
			defer srv.Close()

			p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
			_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测", Speed: tt.in})
			require.NoError(t, err)
			assert.InDelta(t, tt.want, gotBody.Speed, 0.0001)
		})
	}
}

func TestCloudProvider_VolumeMappedToGainDB(t *testing.T) {
	// M1（plan §F）：XTTS volume 是线性 PCM 增益（前端默认 2.0），SiliconFlow gain
	// 是 dB [-10,10]。映射 gain = 20*log10(volume)（clamp），响度与 XTTS 路径对齐；
	// volume<=0 视为未设，不发 gain（上游默认 0）。
	tests := []struct {
		name     string
		volume   float64
		wantGain *float64 // nil = 请求体不含 gain
	}{
		{"volume 2.0 -> +6.02dB（与 XTTS 路径响度对齐）", 2.0, float64Ptr(6.02)},
		{"volume 1.0 -> 0dB", 1.0, float64Ptr(0)},
		{"huge volume clamps to +10dB", 100, float64Ptr(10)},
		{"tiny volume clamps to -10dB", 0.001, float64Ptr(-10)},
		{"unset volume -> no gain field", 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody siliconFlowSpeechReq
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				_, _ = w.Write(buildWav(t, 24000, 1, 16, 240))
			}))
			defer srv.Close()

			p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
			_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测", Volume: tt.volume})
			require.NoError(t, err)
			if tt.wantGain == nil {
				assert.Nil(t, gotBody.Gain, "volume<=0 不得发 gain 字段")
			} else {
				require.NotNil(t, gotBody.Gain)
				assert.InDelta(t, *tt.wantGain, *gotBody.Gain, 0.01)
			}
		})
	}
}

func TestCloudProvider_Name_IsCloud(t *testing.T) {
	p := NewCloudTTSProvider(cloudTestOpts("http://unused"))
	assert.Equal(t, "cloud", p.Name())
}

func TestCloudProvider_MissingKey_ReturnsError(t *testing.T) {
	// 防御性：装配层（NewTTSProvider）在 key 空时根本不构造 cloud provider，
	// 但 provider 自身也必须 fail-fast 而不是发出无鉴权请求。
	opts := cloudTestOpts("http://unused")
	opts.APIKey = ""
	p := NewCloudTTSProvider(opts)
	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api key not configured")
}

func TestCloudProvider_UpstreamError_ReturnsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":20012,"message":"Invalid API key"}`))
	}))
	defer srv.Close()

	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid API key", "上游错误 message 必须透传（降级日志 reason 消费它）")
	assert.Contains(t, err.Error(), "401", "HTTP 状态码必须出现在错误里")
}

func TestCloudProvider_EmptyAudio_ReturnsError(t *testing.T) {
	// 空响应当错误（否则会算出 duration=0 + 空 phonemes ⇒ 前端整段 throw）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(nil)
	}))
	defer srv.Close()

	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestCloudProvider_NonWav_Rejected(t *testing.T) {
	// 非 WAV 响应（如上游改返回 JSON 错误但 200）必须报错，不得当音频透传
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"unexpected":"json"}`))
	}))
	defer srv.Close()

	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.Error(t, err)
}

func float64Ptr(v float64) *float64 { return &v }

// ===== T3：phonemes/duration 复刻（前端硬校验的另一半，plan §B.1/§B.2）=====
//
// XTTS server.py:330-341 的 phonemes 是 per-char 等分纯算术：
//
//	chars = list(text); total = len(audio)/SAMPLE_RATE
//	char_duration = total/len(chars) if chars else 0
//	phonemes = [{char: c, start: round(i*char_duration, 3), duration: round(char_duration, 3)}]
//
// cloud provider 1:1 复刻该算术（口型保真度不降，plan §A.2 H2）。
// duration 的数据源是 **WAV data chunk 字节数**（不是配置值），sampleRate 优先
// 取 fmt chunk 解析值（防上游忽略 sample_rate 参数）。

func TestComputePhonemes_PerCharEqualDivision(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		total  float64
		wantN  int
		want0  XTTSPhoneme // 第一个字符的期望
		wantN1 XTTSPhoneme // 最后一个字符的期望
	}{
		{
			name: "四字均分 1.0s", text: "你好世界", total: 1.0, wantN: 4,
			want0: XTTSPhoneme{Char: "你", Start: 0, Duration: 0.25},
			wantN1: XTTSPhoneme{Char: "界", Start: 0.75, Duration: 0.25},
		},
		{
			name: "单字独占全程", text: "测", total: 0.5, wantN: 1,
			want0:  XTTSPhoneme{Char: "测", Start: 0, Duration: 0.5},
			wantN1: XTTSPhoneme{Char: "测", Start: 0, Duration: 0.5}, // N-1=0，与首字符同位
		},
		{
			name: "三字除不尽 → round 3 位", text: "abc", total: 1.0, wantN: 3,
			want0:  XTTSPhoneme{Char: "a", Start: 0, Duration: 0.333},
			wantN1: XTTSPhoneme{Char: "c", Start: 0.667, Duration: 0.333},
		},
		{
			name: "emoji 按 rune 计数（与 Python list(str) 同语义）", text: "😀😀", total: 1.0, wantN: 2,
			want0:  XTTSPhoneme{Char: "😀", Start: 0, Duration: 0.5},
			wantN1: XTTSPhoneme{Char: "😀", Start: 0.5, Duration: 0.5},
		},
		{
			name: "空文本 → 空 phonemes（不 panic）", text: "", total: 1.0, wantN: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computePhonemes([]rune(tt.text), tt.total)
			require.Len(t, got, tt.wantN)
			if tt.wantN == 0 {
				return
			}
			assert.Equal(t, tt.want0.Char, got[0].Char)
			assert.InDelta(t, tt.want0.Start, got[0].Start, 0.001)
			assert.InDelta(t, tt.want0.Duration, got[0].Duration, 0.001)
			assert.Equal(t, tt.wantN1.Char, got[tt.wantN-1].Char)
			assert.InDelta(t, tt.wantN1.Start, got[tt.wantN-1].Start, 0.001)
			assert.InDelta(t, tt.wantN1.Duration, got[tt.wantN-1].Duration, 0.001)
		})
	}
}

func TestParseWavMeta_ExtractsDataAndFormat(t *testing.T) {
	tests := []struct {
		name             string
		sampleRate       int
		channels         int
		bits             int
		numSamples       int
		wantDurationSec  float64
		wantSampleRate   int
		wantChannels     int
		wantBitsPerSamp  int
	}{
		{"24kHz mono int16 1.0s", 24000, 1, 16, 24000, 1.0, 24000, 1, 16},
		{"44.1kHz stereo 0.5s", 44100, 2, 16, 22050, 0.5, 44100, 2, 16},
		{"24kHz mono 8bit", 24000, 1, 8, 12000, 0.5, 24000, 1, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := parseWavMeta(buildWav(t, tt.sampleRate, tt.channels, tt.bits, tt.numSamples))
			require.NoError(t, err)
			assert.InDelta(t, tt.wantDurationSec, meta.durationSec(), 0.0001)
			assert.Equal(t, tt.wantSampleRate, meta.sampleRate)
			assert.Equal(t, tt.wantChannels, meta.channels)
			assert.Equal(t, tt.wantBitsPerSamp, meta.bitsPerSample)
		})
	}
}

func TestParseWavMeta_ToleratesExtraChunksBeforeData(t *testing.T) {
	// 真实上游可能在 fmt 与 data 之间插 LIST/fact 等块——解析必须按块遍历而非
	// 假设 data 固定偏移 44。
	base := buildWav(t, 24000, 1, 16, 24000) // data @44, 48000 bytes
	extra := []byte("LIST")
	extraSize := []byte{4, 0, 0, 0}
	extraPayload := []byte("INFO")
	withExtra := append(append(append([]byte{}, base[:36]...), extra...), extraSize...)
	withExtra = append(withExtra, extraPayload...)
	// 重新补 RIFF size 与 data 块（简化：直接拼 data 头+体）
	dataHdr := base[36:44] // "data" + size
	withExtra = append(withExtra, dataHdr...)
	withExtra = append(withExtra, make([]byte, 48000)...)

	meta, err := parseWavMeta(withExtra)
	require.NoError(t, err)
	assert.InDelta(t, 1.0, meta.durationSec(), 0.0001, "data 字节数必须按块定位，不受前置块影响")
}

func TestParseWavMeta_RejectsNonWav(t *testing.T) {
	_, err := parseWavMeta([]byte(`{"not":"wav"}`))
	require.Error(t, err)

	_, err = parseWavMeta(nil)
	require.Error(t, err)
}

func TestParseWavMeta_StreamSentinelDataSize_UsesActualBytes(t *testing.T) {
	// F-198 运行时实测（2026-10-07）：SiliconFlow 的 WAV 响应 data 块声明
	// 0xFFFFFF00（流式/未知长度哨兵），实际字节完整到港。此时必须以实际字节
	// 算 duration——原"声明>实际即报错"语义会把 cloud 路径整体打成降级
	//（实测 BFF Warn: declared 4294967040 bytes, have 307356）。
	// data 声明值不可信是上游契约的一部分；fmt 块仍须完整（那才是格式事实源）。
	wav := buildWav(t, 24000, 1, 16, 24000) // 1.0s / 48000 data bytes
	// 把 data 块的 size 字段改写为哨兵 0xFFFFFF00
	sentinel := []byte{0x00, 0xFF, 0xFF, 0xFF}
	copy(wav[40:44], sentinel) // "data" @36..39，size @40..43

	meta, err := parseWavMeta(wav)
	require.NoError(t, err, "哨兵 size 不得按截断报错")
	assert.InDelta(t, 1.0, meta.durationSec(), 0.001, "duration 以实际到港字节计算")
}

func TestParseWavMeta_RejectsTruncatedData(t *testing.T) {
	// data 声明 48000 字节但实际只到港一半：仍以实际字节算（同哨兵语义——
	// 无法区分"流式哨兵"与"真截断"，取实际值让播放层自然收尾）；
	// 但 fmt 块截断必须报错（TestParseWavMeta_RejectsTruncatedFmt）。
	wav := buildWav(t, 24000, 1, 16, 24000)
	truncated := wav[:len(wav)/2] // data 只到港一半
	meta, err := parseWavMeta(truncated)
	require.NoError(t, err, "data 短到港以实际字节计算，不报错")
	assert.Greater(t, meta.durationSec(), 0.0)
}

func TestParseWavMeta_RejectsTruncatedFmt(t *testing.T) {
	// fmt 块声明 16 字节但只到港 8 → 报错（fmt 是格式事实源，截断不可解析）
	wav := buildWav(t, 24000, 1, 16, 24000)
	truncated := wav[:36] // RIFF/WAVE + "fmt " 头，fmt body 全缺
	_, err := parseWavMeta(truncated)
	require.Error(t, err, "fmt 块截断必须报错")
}

func TestCloudProvider_FillsPhonemesAndDuration(t *testing.T) {
	// 集成：provider 返回的响应必须同时满足前端三项硬校验
	// （useTTSPlayer.ts:215 audio/phonemes/duration，缺一整段 throw）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(buildWav(t, 24000, 1, 16, 24000)) // 1.0s
	}))
	defer srv.Close()

	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "你好世界", Language: "zh-cn"})
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Audio, "硬校验 1/3：audio")
	assert.InDelta(t, 1.0, resp.Duration, 0.001, "硬校验 3/3：duration 来自 WAV data 字节数")
	require.Len(t, resp.Phonemes, 4, "硬校验 2/3：phonemes per-char")
	assert.Equal(t, "你", resp.Phonemes[0].Char)
	assert.InDelta(t, 0.25, resp.Phonemes[3].Duration, 0.001)
}

func TestCloudProvider_DurationFromParsedSampleRate(t *testing.T) {
	// 上游若忽略 sample_rate 参数返回 44.1k，duration 必须按实际 fmt 算
	// （44100Hz mono 16bit × 22050 samples = 0.5s），而不是按配置的 24000。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buildWav(t, 44100, 1, 16, 22050))
	}))
	defer srv.Close()

	p := NewCloudTTSProvider(cloudTestOpts(srv.URL))
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.NoError(t, err)
	assert.InDelta(t, 0.5, resp.Duration, 0.001, "duration 必须来自 fmt chunk 实际采样率")
	assert.Equal(t, 44100, resp.SampleRate, "SampleRate 回填实际值（前端 WAV 头/播放以此为准）")
}

