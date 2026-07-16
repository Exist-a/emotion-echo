package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"emotion-echo-gin/internal/config"
)

type ASRClient struct {
	cfg        *config.Config
	httpClient *http.Client
	baseURL    string
}

func NewASRClient(cfg *config.Config) *ASRClient {
	baseURL := cfg.AI.ASR.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8001"
	}

	return &ASRClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		baseURL: baseURL,
	}
}

type ASRResult struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

func (c *ASRClient) Transcribe(ctx context.Context, audioFilePath string) (*ASRResult, error) {
	file, err := os.Open(audioFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	url := c.baseURL + "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ASR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ASR API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ASR response: %w", err)
	}

	return &ASRResult{
		Text: result.Text,
	}, nil
}

func (c *ASRClient) TranscribeFromURL(ctx context.Context, audioURL string) (*ASRResult, error) {
	reqBody := map[string]interface{}{
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "audio_url",
						"audio_url": map[string]string{
							"url": audioURL,
						},
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ASR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ASR API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ASR response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no response from ASR")
	}

	content := result.Choices[0].Message.Content

	var parsed struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		return &ASRResult{
			Text:     parsed.Text,
			Language: parsed.Language,
		}, nil
	}

	return &ASRResult{
		Text: content,
	}, nil
}
