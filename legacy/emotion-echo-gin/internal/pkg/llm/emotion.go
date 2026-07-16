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

type EmotionClient struct {
	cfg        *config.Config
	httpClient *http.Client
	baseURL    string
}

func NewEmotionClient(cfg *config.Config) *EmotionClient {
	baseURL := cfg.AI.Emotion.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8002"
	}

	return &EmotionClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		baseURL: baseURL,
	}
}

type EmotionResult struct {
	Emotion   string  `json:"emotion"`
	Confidence float64 `json:"confidence"`
	Text      string  `json:"text"`
}

var emotionLabelMap = map[string]string{
	"happy":   "开心",
	"sad":     "悲伤",
	"angry":   "愤怒",
	"anxious": "焦虑",
	"fear":    "恐惧",
	"neutral": "中性",
	"unk":     "未知",
	"unknown": "未知",
}

func (c *EmotionClient) AnalyzeEmotion(ctx context.Context, audioFilePath string) (*EmotionResult, error) {
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

	url := c.baseURL + "/analyze"
	fmt.Printf("[DEBUG] Calling SenseVoice API: POST %s\n", url)
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("emotion analysis request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("emotion API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	fmt.Printf("[DEBUG] SenseVoice API response status: %d\n", resp.StatusCode)
	var result EmotionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode emotion response: %w", err)
	}

	fmt.Printf("[DEBUG] Raw API response - emotion: \"%s\", text: \"%s\", confidence: %.2f\n", 
		result.Emotion, result.Text, result.Confidence)

	if result.Emotion == "" {
		fmt.Println("[DEBUG] Emotion is empty, defaulting to \"neutral\"")
		result.Emotion = "neutral"
	}

	if label, ok := emotionLabelMap[result.Emotion]; ok {
		fmt.Printf("[DEBUG] Emotion mapping: \"%s\" -> \"%s\"\n", result.Emotion, label)
	} else {
		fmt.Printf("[DEBUG] Unknown emotion: \"%s\", will use as-is\n", result.Emotion)
	}

	return &result, nil
}

func GetEmotionLabel(en string) string {
	if label, ok := emotionLabelMap[en]; ok {
		return label
	}
	return en
}
