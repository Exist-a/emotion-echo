package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"emotion-echo-gin/internal/config"
)

// Client LLM客户端
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewClient 创建LLM客户端
func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Call 调用LLM
func (c *Client) Call(ctx context.Context, prompt string) (string, error) {
	// 选择配置（仅支持 Kimi）
	apiKey := c.cfg.AI.Kimi.APIKey
	baseURL := c.cfg.AI.Kimi.BaseURL
	if baseURL == "" {
		baseURL = "https://api.moonshot.cn/v1"
	}
	model := c.cfg.AI.Kimi.Model
	if model == "" {
		model = "moonshot-v1-8k"
	}
	
	// 构建请求
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 1000,
		"temperature": 0.3,
	}
	
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	
	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM API error: %s", string(body))
	}
	
	// 解析响应
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}
	
	return result.Choices[0].Message.Content, nil
}
