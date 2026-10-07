// Package downstream — tts_provider.go
//
// E2E-F-198（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §C）：TTS 上游
// provider 接口隔离——在线主链路改接 CosyVoice2 API（cloud），XTTS 降为离线
// 回退（local）。ADR：docs/architecture/adr/adr-2026-10-tts-api-cosyvoice2.md
// （架构决策 40 / D-44）。
//
// 硬约束（plan §B，读码得出，违反任一 = 前端整段静默丢弃）：
//   - 统一返回类型复用 XTTSPhonemesResp：前端 useTTSPlayer.ts:215 对
//     audio/phonemes/duration 三项联合硬校验，缺一即 throw ⇒ cloud provider
//     必须在 BFF 侧自算 phonemes 与 duration（XTTS server.py:330-332 本就是
//     per-char 等分纯算术，1:1 复刻、口型保真度不降）
//   - 禁止业务代码直写供应商域名/模型名：一律经 config 注入（ADR §Decision.1）
package downstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

// TTSProvider 是 TTS 上游的统一抽象（cloud=CosyVoice2 API / local=XTTS）。
//
// Synthesize 复用 XTTSPhonemesResp 作为统一返回类型是刻意选择：前端契约
// （/api/v1/tts/phonemes 的 JSON 信封）与 handler 封装零改动，改动面收敛在
// downstream 层。类型名后续可重命名为 TTSSynthResp（不改 JSON 字段）。
type TTSProvider interface {
	// Synthesize 合成语音并返回 base64 WAV + phonemes + duration
	Synthesize(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error)
	// Name 供降级日志与指标 label 消费（"cloud" / "local"）
	Name() string
}

// localTTSProvider 包装既有 XTTSClient —— 纯转调，零行为变更（回退路径）。
type localTTSProvider struct {
	xtts XTTSClient
}

// NewLocalTTSProvider 构造 local provider（xtts 可为已配置的 XTTSClient）。
func NewLocalTTSProvider(xtts XTTSClient) TTSProvider {
	return &localTTSProvider{xtts: xtts}
}

func (p *localTTSProvider) Synthesize(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	return p.xtts.Phonemes(ctx, req)
}

func (p *localTTSProvider) Name() string { return "local" }

// ===== cloud provider（SiliconFlow /v1/audio/speech，CosyVoice2-0.5B）=====

// CloudTTSOptions 构造选项。所有字段经 config 注入——**禁止在调用方硬编码
// 供应商域名/模型名**（ADR §Decision.1：换供应商只改配置）。
type CloudTTSOptions struct {
	// BaseURL 含 /v1 前缀（经 config/env 注入，具体值见 etc/web-bff.yaml TTS 段
	// 与 SetDefaults 注释），provider 只补 /audio/speech
	BaseURL    string
	APIKey     string
	Model      string
	Voice      string // 预置音色需带模型名前缀（官方文档约定）
	SampleRate int    // 必须 24000（官方默认 44100，与 XTTS SAMPLE_RATE 对齐）
	TimeoutMs  int
}

// cloudSpeechReq 是 /audio/speech 请求体（字段名与官方文档一一对应）。
type cloudSpeechReq struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format"`
	SampleRate     int      `json:"sample_rate"`
	Speed          float64  `json:"speed"`
	// Gain 是 dB 域（[-10,10]）；volume<=0 时不发（上游默认 0）
	Gain *float64 `json:"gain,omitempty"`
}

type cloudTTSProvider struct {
	opts CloudTTSOptions
	http *http.Client
}

// NewCloudTTSProvider 构造 cloud provider。
func NewCloudTTSProvider(opts CloudTTSOptions) TTSProvider {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		// plan §C：cloud 实测 33 字完整返回 ~1.5s，30s 极宽裕（兜底防呆）
		timeout = 30 * time.Second
	}
	return &cloudTTSProvider{opts: opts, http: &http.Client{Timeout: timeout}}
}

func (p *cloudTTSProvider) Name() string { return "cloud" }

func (p *cloudTTSProvider) Synthesize(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	if p.opts.APIKey == "" {
		return nil, fmt.Errorf("downstream: tts cloud: api key not configured")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("downstream: tts cloud: text is required")
	}

	payload, err := json.Marshal(cloudSpeechReq{
		Model:          p.opts.Model,
		Input:          req.Text,
		Voice:          p.opts.Voice,
		ResponseFormat: "wav", // plan §B.3：前端 base64ToWavBlob 不构造 WAV 头
		SampleRate:     p.opts.SampleRate,
		Speed:          clampSpeed(req.Speed),
		Gain:           volumeToGainDB(req.Volume),
	})
	if err != nil {
		return nil, fmt.Errorf("downstream: tts cloud: marshal req: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.opts.BaseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("downstream: tts cloud: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.opts.APIKey)

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: tts cloud: do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var errBody struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		}
		_ = json.Unmarshal(raw, &errBody)
		msg := errBody.Message
		if msg == "" {
			msg = errBody.Detail
		}
		if msg == "" {
			msg = string(raw)
		}
		return nil, fmt.Errorf("downstream: tts cloud: status %d: %s", resp.StatusCode, msg)
	}

	audio, err := io.ReadAll(io.LimitReader(resp.Body, maxCloudAudioBytes))
	if err != nil {
		return nil, fmt.Errorf("downstream: tts cloud: read audio: %w", err)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("downstream: tts cloud: empty audio response")
	}
	if !isWavContainer(audio) {
		return nil, fmt.Errorf("downstream: tts cloud: response is not a WAV container (%d bytes)", len(audio))
	}

	// 前端硬校验（useTTSPlayer.ts:215）三件套的另外两件：duration + phonemes。
	// duration/采样率以 WAV 实际解析值为准（防上游忽略 sample_rate 参数）。
	meta, err := parseWavMeta(audio)
	if err != nil {
		return nil, fmt.Errorf("downstream: tts cloud: %w", err)
	}
	duration := meta.durationSec()

	return &XTTSPhonemesResp{
		Audio:      base64.StdEncoding.EncodeToString(audio),
		SampleRate: meta.sampleRate,
		Text:       req.Text,
		Language:   req.Language,
		Phonemes:   computePhonemes([]rune(req.Text), duration),
		Duration:   round3(duration),
	}, nil
}

// maxCloudAudioBytes 防-responsive 超大响应：60s @24kHz 16bit mono ≈ 2.9MB，
// 取 16MB 上限（容错双声道/44.1k 异常配置），超限按错误处理。
const maxCloudAudioBytes = 16 << 20

// isWavContainer 检查 RIFF....WAVE 头。
func isWavContainer(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WAVE"))
}

// clampSpeed 把 XTTS 语义的 speed 映射进 SiliconFlow 合法域 [0.25, 4.0]；
// 未设（<=0）用上游默认 1.0（XTTS 默认 0.75 是显式传值，不受此影响）。
func clampSpeed(s float64) float64 {
	switch {
	case s <= 0:
		return 1.0
	case s < 0.25:
		return 0.25
	case s > 4.0:
		return 4.0
	default:
		return s
	}
}

// volumeToGainDB 把 XTTS 的线性 PCM 增益（前端默认 2.0）映射为 SiliconFlow 的
// dB 域 [-10,10]：gain = 20*log10(volume)，clamp。volume<=0 视为未设（不发
// gain 字段）。选此映射是为与 XTTS 路径主观响度对齐（plan §F M1-B）。
func volumeToGainDB(volume float64) *float64 {
	if volume <= 0 {
		return nil
	}
	g := 20 * math.Log10(volume)
	if g > 10 {
		g = 10
	}
	if g < -10 {
		g = -10
	}
	return &g
}

// ===== WAV 解析 + phonemes 复刻（前端硬校验的另一半，plan §B.1/§B.2）=====

// wavMeta 是从 WAV 容器解析出的关键元数据。
type wavMeta struct {
	dataBytes     int
	sampleRate    int
	channels      int
	bitsPerSample int
}

// durationSec 由 data 字节数与 fmt 格式算出实际时长（int16/8bit 通用）。
func (m wavMeta) durationSec() float64 {
	bytesPerSec := m.sampleRate * m.channels * (m.bitsPerSample / 8)
	if bytesPerSec <= 0 {
		return 0
	}
	return float64(m.dataBytes) / float64(bytesPerSec)
}

// parseWavMeta 按块遍历 RIFF/WAVE 容器提取 fmt + data 元数据。
//
// 不假设 data 固定偏移 44——真实上游可能在 fmt 与 data 之间插 LIST/fact 等块；
// data 声明尺寸与实际不符时报错（不得静默算出错误 duration）。
func parseWavMeta(wav []byte) (wavMeta, error) {
	if !isWavContainer(wav) {
		return wavMeta{}, fmt.Errorf("downstream: tts cloud: not a WAV container")
	}
	var meta wavMeta
	haveFmt := false
	haveData := false
	pos := 12 // 跳过 RIFF size + "WAVE"
	for pos+8 <= len(wav) {
		id := string(wav[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(wav[pos+4 : pos+8]))
		bodyStart := pos + 8
		if bodyStart+size > len(wav) {
			return wavMeta{}, fmt.Errorf("downstream: tts cloud: wav chunk %q truncated (declared %d bytes, have %d)", id, size, len(wav)-bodyStart)
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return wavMeta{}, fmt.Errorf("downstream: tts cloud: wav fmt chunk too small (%d bytes)", size)
			}
			meta.channels = int(binary.LittleEndian.Uint16(wav[bodyStart+2:]))
			meta.sampleRate = int(binary.LittleEndian.Uint32(wav[bodyStart+4:]))
			meta.bitsPerSample = int(binary.LittleEndian.Uint16(wav[bodyStart+14:]))
			haveFmt = true
		case "data":
			meta.dataBytes = size
			haveData = true
		}
		pos = bodyStart + size
		if size%2 == 1 {
			pos++ // RIFF 块按字对齐
		}
	}
	if !haveFmt || !haveData || meta.sampleRate == 0 || meta.channels == 0 || meta.bitsPerSample == 0 {
		return wavMeta{}, fmt.Errorf("downstream: tts cloud: wav missing fmt/data chunk or invalid format")
	}
	return meta, nil
}

// computePhonemes per-char 等分——1:1 复刻 XTTS server.py:330-341 的算术
// （XTTS 本身就不是真字符级时间戳，见 xtts.go XTTSPhoneme 注释；复刻等分即
// 保真度等价）。start/duration round 到 3 位，与 Python round(x, 3) 同精度。
func computePhonemes(chars []rune, totalSec float64) []XTTSPhoneme {
	n := len(chars)
	if n == 0 {
		return []XTTSPhoneme{}
	}
	charDur := totalSec / float64(n)
	out := make([]XTTSPhoneme, n)
	for i, c := range chars {
		out[i] = XTTSPhoneme{
			Char:     string(c),
			Start:    round3(float64(i) * charDur),
			Duration: round3(charDur),
		}
	}
	return out
}

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }

// ===== 回退链与装配（plan §B.5 语义矩阵）=====

// TTSProviderConfig 是 provider 装配入参——把 mode/key 语义集中在 downstream
// 层，main.go 不散落 if 分支。Cloud/Local 均为接口注入（可测试性原则 §3.1）。
type TTSProviderConfig struct {
	// Mode: "auto"（默认，空串同 auto）| "cloud" | "local"
	Mode   string
	APIKey string
	// Cloud 可为 nil（auto+key 空 / local 模式用不到）
	Cloud TTSProvider
	// Local 必填（XTTS 离线回退）
	Local TTSProvider
	// Logger 缺省 slog.Default()
	Logger *slog.Logger
	// OnFallback 降级钩子（供 main 接 metrics 计数；reason 为可读错误串）
	OnFallback func(reason string)
}

// NewTTSProvider 按 plan §B.5 语义矩阵装配：
//
//	mode=auto + key 空 → 直接 local（dev 常态，无降级包装 ⇒ 无 Warn）
//	mode=auto + key 有 → fallbackProvider{cloud → local}，降级打 Warn(reason)
//	mode=cloud         → 只用 cloud，失败即失败（fail-loud，实测甄别用）
//	mode=local         → 只用 XTTS（回滚开关 / 离线环境）
func NewTTSProvider(cfg TTSProviderConfig) TTSProvider {
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "cloud":
		return cfg.Cloud
	case "local":
		return cfg.Local
	default:
		if cfg.APIKey == "" || cfg.Cloud == nil {
			return cfg.Local
		}
		logger := cfg.Logger
		if logger == nil {
			logger = slog.Default()
		}
		return &fallbackTTSProvider{
			primary:    cfg.Cloud,
			fallback:   cfg.Local,
			logger:     logger,
			onFallback: cfg.OnFallback,
		}
	}
}

// fallbackTTSProvider cloud 失败（key 语义上已就位）→ 降级 local，且**必须可观测**：
// Warn 带 reason + OnFallback 钩子。"云端一直挂但没人知道" = E2E-22 抓过的静默失效同型。
type fallbackTTSProvider struct {
	primary    TTSProvider
	fallback   TTSProvider
	logger     *slog.Logger
	onFallback func(reason string)
}

func (p *fallbackTTSProvider) Synthesize(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	resp, err := p.primary.Synthesize(ctx, req)
	if err == nil {
		return resp, nil
	}
	reason := err.Error()
	p.logger.WarnContext(ctx, "tts cloud degraded to local",
		"primary", p.primary.Name(), "fallback", p.fallback.Name(), "reason", reason)
	if p.onFallback != nil {
		p.onFallback(reason)
	}
	return p.fallback.Synthesize(ctx, req)
}

func (p *fallbackTTSProvider) Name() string { return "auto" }
