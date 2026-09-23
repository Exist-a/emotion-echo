// Package downstream — xtts.go
//
// Stage 30 / stage-30-web-bff.md T2.9-11: XTTSClient（BFF → XTTS 直连）
//
// XTTS（emotion-echo-models/XTTS，FastAPI :8003）：
//   POST /tts_stream → 流式 raw WAV/PCM 字节（media_type audio/wav）
//   GET  /health      → {"status","model_loaded","model_type"}
//
// 无鉴权（调研确认 XTTS 不挂 GinAuthMiddleware）。
// 流式语义：client 返回 io.ReadCloser，由 BFF 的 tts_stream handler 逐块转发。
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TTSStreamReq 流式 TTS 请求
type TTSStreamReq struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
}

// XTTSHealthResp 对应 XTTS /health 响应
type XTTSHealthResp struct {
	Status     string `json:"status"`
	ModelLoaded bool  `json:"model_loaded"`
	ModelType  string `json:"model_type"`
}

// TTSPhonemesReq 请求体（与 TTSStreamReq 同形 —— 仓 XTTS /tts_with_phonemes 用
// 同样的 TTSRequest schema；独立类型便于未来扩展）
type TTSPhonemesReq struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
}

// XTTSPhoneme 单字符时间戳（仓 server.py:319-326 per-char 等分近似）
//
// ⚠️ 语义限制（E2E-17 plan §2.A.3）：start 是**秒**（不是 ms）；
// duration 是 per-char 等分（total_duration / len(chars)），**不是真 XTTS 字符级
// 时间戳推理**。D-03 "对齐"目标能达成，"真字符读音时机"做不到。report.md
// 必须明确此口径。
type XTTSPhoneme struct {
	Char     string  `json:"char"`
	Start    float64 `json:"start"`     // 秒
	Duration float64 `json:"duration"`  // 秒
}

// XTTSPhonemesResp 对应 XTTS /tts_with_phonemes 响应
type XTTSPhonemesResp struct {
	Audio      string         `json:"audio"`       // base64-encoded WAV
	SampleRate int            `json:"sample_rate"`
	Text       string         `json:"text"`
	Language   string         `json:"language"`
	Phonemes   []XTTSPhoneme  `json:"phonemes"`
	Duration   float64        `json:"duration"`    // 秒
}

// XTTSClient BFF → XTTS 客户端（直连，不经 ai-svc）
type XTTSClient interface {
	// Stream 发起流式 TTS，返回 raw WAV 字节流（调用方负责 Close）
	Stream(ctx context.Context, req TTSStreamReq) (io.ReadCloser, error)
	// Health 查询模型健康状态
	Health(ctx context.Context) (*XTTSHealthResp, error)
	// Phonemes 发起带字符级时间戳的 TTS，返回 JSON（含 base64 audio + phonemes 数组）。
	// E2E-17 D-03 真口型同步：本接口供应前端 useTTSPlayer 按 currentTime 驱动 BlendShape。
	// nil-safe：未配置时返回 "xtts client not configured" 错误（与 Stream/Health 同款纪律）。
	Phonemes(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error)
}

// XTTSClientOptions 构造选项
type XTTSClientOptions struct {
	BaseURL   string
	TimeoutMs int
}

// xttsHTTPClient 是 XTTSClient 的 HTTP 实现
type xttsHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewXTTSClient 构造 XTTSClient
func NewXTTSClient(opts XTTSClientOptions) XTTSClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		// E2E-F-127：30s 在 dev CPU 上不够（仓 XTTS 模型 CPU 推理 + 字符级时间戳计算
		// 实测 4 字符 ~7s，按长度线性放大；保守 90s 覆盖 50+ 字符）。
		// 注：E2E-F-115 已把 BFF→ai-svc deadline 从 5s 抬到 30s；本客户端调的是
		// XTTS 直连（不走 ai-svc），原本 30s 在 Phonemes 路径实测不够。
		timeout = 90 * time.Second
	}
	return &xttsHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *xttsHTTPClient) Stream(ctx context.Context, req TTSStreamReq) (io.ReadCloser, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal tts stream req: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tts_stream", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: xtts stream: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Detail != "" {
			return nil, fmt.Errorf("downstream: xtts stream: %s", body.Detail)
		}
		return nil, fmt.Errorf("downstream: xtts stream: unexpected status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (c *xttsHTTPClient) Health(ctx context.Context) (*XTTSHealthResp, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("downstream: xtts client not configured")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: xtts health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("downstream: xtts health: unexpected status %d", resp.StatusCode)
	}
	var health XTTSHealthResp
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("downstream: xtts health decode: %w", err)
	}
	return &health, nil
}

// Phonemes 调用 XTTS /tts_with_phonemes，返回含字符级时间戳的 JSON 响应。
// 镜像 Stream 的 nil-safe + 错误透传（FastAPI {"detail":"..."} 提取）。
func (c *xttsHTTPClient) Phonemes(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("downstream: xtts client not configured")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal tts phonemes req: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tts_with_phonemes", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: xtts phonemes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Detail != "" {
			return nil, fmt.Errorf("downstream: xtts phonemes: %s", body.Detail)
		}
		return nil, fmt.Errorf("downstream: xtts phonemes: unexpected status %d", resp.StatusCode)
	}
	var out XTTSPhonemesResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: xtts phonemes decode: %w", err)
	}
	return &out, nil
}
