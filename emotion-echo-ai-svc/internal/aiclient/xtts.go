package aiclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// XTTSClient 语音克隆 TTS 客户端（Stage 22-A.3）。
//
// backend: emotion-echo-models/XTTS -> FastAPI :8003/tts  (完整)  /tts_stream  (流式)
//
// 完整模式：返回 base64 WAV，适合 < 100 字的合成
// 流式模式：返回 raw bytes，适合长文本
type XTTSClient struct {
	baseURL string
	hc      *http.Client
	timeout time.Duration

	// TimeoutFn 是超时的运行时取值钩子（E2E-23 #31 / D-32）。
	//
	// 非 nil 时**每请求**调它取当前值；nil 时退回构造期冻结的 c.timeout。
	// 用途：Nacos ops 热更 —— 此前 timeout 在构造期拷贝，改配置对已运行的
	// 实例完全无效（与 F-159 的 LLM.Timeout 同型，"配了不生效"）。
	//
	// 用函数钩子而非直接依赖 ops 包：aiclient 是 logic 层的底层依赖，
	// 反向 import ops 会造成不必要的耦合；钩子把"值从哪来"留给装配层。
	TimeoutFn func() time.Duration
	// LanguageFn / SpeedFn 是语种与语速的运行时取值钩子（E2E-23 #31）。
	// 非 nil 时每请求调它；nil 时退回构造期值。
	// 换语种/调语速不需要重启服务。
	LanguageFn func() string
	SpeedFn    func() float64
	language   string
	speed      float64
}

// TTSRequest 客户端发到 /tts 的请求结构
type TTSRequest struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Speed    float64 `json:"speed"`
}

// TTSResponse 服务端 /tts 返回的标准化结果
type TTSResponse struct {
	Audio      string `json:"audio"` // base64-encoded WAV
	SampleRate int    `json:"sample_rate"`
	Text       string `json:"text"`
	Language   string `json:"language"`
}

// NewXTTSClient 构造 XTTS 客户端，baseURL 空时返回 nil。
func NewXTTSClient(c Config, language string, speed float64) *XTTSClient {
	if c.BaseURL == "" {
		return nil
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	if language == "" {
		language = "zh-cn"
	}
	if speed <= 0 {
		speed = 0.75
	}
	return &XTTSClient{
		baseURL:  c.BaseURL,
		hc:       &http.Client{Timeout: defaultHTTPTimeout},
		timeout:  time.Duration(timeout) * time.Second,
		language: language,
		speed:    speed,
	}
}

// Synthesize 完整合成 → 返回 WAV bytes + sample_rate
func (c *XTTSClient) Synthesize(ctx context.Context, text string) ([]byte, int, error) {
	if c == nil {
		return nil, 0, ErrNotConfigured
	}
	if strings.TrimSpace(text) == "" {
		return nil, 0, fmt.Errorf("empty text")
	}

	payload := TTSRequest{
		Text:     text,
		Language: c.effectiveLanguage(),
		Speed:    c.effectiveSpeed(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tts", bytes.NewReader(raw))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("call XTTS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, &ErrUpstream{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var out TTSResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, 0, fmt.Errorf("decode XTTS response: %w", err)
	}
	if out.Audio == "" {
		return nil, 0, fmt.Errorf("empty audio in response")
	}
	wavBytes, err := base64.StdEncoding.DecodeString(out.Audio)
	if err != nil {
		return nil, 0, fmt.Errorf("decode base64 audio: %w", err)
	}
	return wavBytes, out.SampleRate, nil
}

// SynthesizeToWAV synthesizes text and returns raw WAV bytes; a thin helper over Synthesize.
func (c *XTTSClient) SynthesizeToWAV(ctx context.Context, text string) ([]byte, error) {
	wav, _, err := c.Synthesize(ctx, text)
	return wav, err
}

// Health /health 探活
func (c *XTTSClient) Health(ctx context.Context) error {
	if c == nil {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &ErrUpstream{StatusCode: resp.StatusCode}
	}
	return nil
}

// effectiveTimeout 返回本次请求应使用的超时（E2E-23 #31）。
func (c *XTTSClient) effectiveTimeout() time.Duration {
	if c.TimeoutFn != nil {
		if d := c.TimeoutFn(); d > 0 {
			return d
		}
	}
	return c.timeout
}

// effectiveLanguage 返回本次请求应使用的语种（E2E-23 #31）。
func (c *XTTSClient) effectiveLanguage() string {
	if c.LanguageFn != nil {
		if v := c.LanguageFn(); v != "" {
			return v
		}
	}
	return c.language
}

// effectiveSpeed 返回本次请求应使用的语速（E2E-23 #31）。
func (c *XTTSClient) effectiveSpeed() float64 {
	if c.SpeedFn != nil {
		if v := c.SpeedFn(); v > 0 {
			return v
		}
	}
	return c.speed
}
