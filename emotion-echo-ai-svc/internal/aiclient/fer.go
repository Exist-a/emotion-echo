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

// FERClient 面部表情识别客户端（Stage 22-A.1）。
//
// backend: Emotion-Echo-LLM/FER -> FastAPI :8004/analyze
//
// 用法：
//   client := aiclient.NewFERClient(cfg.FER)
//   if client == nil { /* service 关闭，走降级 */ }
//   result, err := client.AnalyzeImage(ctx, imageBytes, "face.jpg")
type FERClient struct {
	baseURL string
	hc      *http.Client
	timeout time.Duration
}

// FERResult 服务端 /analyze 返回的标准化结果
type FERResult struct {
	Emotion    string             `json:"emotion"`
	Confidence float64            `json:"confidence"`
	Scores     map[string]float64 `json:"scores"`
	Source     string             `json:"source"`
}

// NewFERClient 返回 FER HTTP 客户端。baseURL 空时返回 nil。
func NewFERClient(c Config) *FERClient {
	if c.BaseURL == "" {
		return nil
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10
	}
	return &FERClient{
		baseURL: c.BaseURL,
		hc:      &http.Client{Timeout: defaultHTTPTimeout},
		timeout: time.Duration(timeout) * time.Second,
	}
}

// AnalyzeImage 上传图片 → 返回情绪分析结果
func (c *FERClient) AnalyzeImage(ctx context.Context, imageBytes []byte, filename string) (*FERResult, error) {
	if c == nil {
		return nil, ErrNotConfigured
	}
	if len(imageBytes) == 0 {
		return nil, fmt.Errorf("empty image bytes")
	}

	// multipart form: file=...
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		return nil, fmt.Errorf("write image: %w", err)
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
		return nil, fmt.Errorf("call FER: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read FER response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ErrUpstream{StatusCode: resp.StatusCode, Body: string(raw)}
	}

	var out FERResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode FER response: %w", err)
	}
	return &out, nil
}

// Health /health 探活
func (c *FERClient) Health(ctx context.Context) error {
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

// Config 来自 config.Config.FER（避免循环 import）
type Config struct {
	BaseURL string
	Timeout int
}
