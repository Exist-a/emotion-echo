package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// SenseVoiceClient 语音 ASR + 情绪识别客户端（Stage 22-A.2）。
//
// backend: Emotion-Echo-LLM/sensevoice-small -> FastAPI :8002/analyze
type SenseVoiceClient struct {
	baseURL string
	hc      *http.Client
	timeout time.Duration
}

// SenseVoiceResult 服务端 /analyze 返回的标准化结果
type SenseVoiceResult struct {
	Text       string  `json:"text"`
	Emotion    string  `json:"emotion"`
	Confidence float64 `json:"confidence"`
	RawText    string  `json:"raw_text"`
	Source     string  `json:"source"`
}

// NewSenseVoiceClient 构造 SenseVoice 客户端，baseURL 空时返回 nil。
func NewSenseVoiceClient(c Config) *SenseVoiceClient {
	if c.BaseURL == "" {
		return nil
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	return &SenseVoiceClient{
		baseURL: c.BaseURL,
		hc:      &http.Client{Timeout: defaultHTTPTimeout},
		timeout: time.Duration(timeout) * time.Second,
	}
}

// Analyze 上传音频 → 返回转写文本 + 情绪分析
func (c *SenseVoiceClient) Analyze(ctx context.Context, audioBytes []byte, filename string) (*SenseVoiceResult, error) {
	if c == nil {
		return nil, ErrNotConfigured
	}
	if len(audioBytes) == 0 {
		return nil, fmt.Errorf("empty audio bytes")
	}

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(audioBytes); err != nil {
		return nil, fmt.Errorf("write audio: %w", err)
	}
	mw.Close()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/analyze", body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call SenseVoice: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read SenseVoice response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ErrUpstream{StatusCode: resp.StatusCode, Body: string(raw)}
	}

	var out SenseVoiceResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode SenseVoice response: %w", err)
	}
	return &out, nil
}

// Health /health 探活
func (c *SenseVoiceClient) Health(ctx context.Context) error {
	if c == nil {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
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
