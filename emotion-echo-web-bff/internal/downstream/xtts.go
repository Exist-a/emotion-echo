// Package downstream — xtts.go
//
// Stage 30 / stage-30-web-bff.md T2.9-11: XTTSClient（BFF → XTTS 直连）
//
// XTTS（Emotion-Echo-LLM/XTTS，FastAPI :8003）：
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

// XTTSClient BFF → XTTS 客户端（直连，不经 ai-svc）
type XTTSClient interface {
	// Stream 发起流式 TTS，返回 raw WAV 字节流（调用方负责 Close）
	Stream(ctx context.Context, req TTSStreamReq) (io.ReadCloser, error)
	// Health 查询模型健康状态
	Health(ctx context.Context) (*XTTSHealthResp, error)
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
		timeout = 30 * time.Second
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
	var out XTTSHealthResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode xtts health: %w", err)
	}
	return &out, nil
}
