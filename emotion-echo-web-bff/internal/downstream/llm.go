// Package downstream — llm.go
//
// Stage 30 / Phase D: OpenAI 兼容 LLM 客户端（DeepSeek / OpenAI / Kimi / 本地 Ollama）。
//
// 调用 /v1/chat/completions，支持流式（stream:true 返回 SSE，data: {choices:[{delta:{content}}]}）。
// base_url 指向 DeepSeek / OpenAI 兼容服务。
package downstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMOptions 构造选项
type LLMOptions struct {
	BaseURL string // e.g. "https://api.deepseek.com" / "https://api.openai.com" / "http://localhost:11434"（Ollama）
	APIKey  string // sk-...（Ollama 留空）
	Model   string // "deepseek-chat" / "gpt-4o-mini" / "qwen-turbo" / "llama3"
	Timeout time.Duration
}

// LLMChatReq OpenAI 兼容 chat.completions 请求
type LLMChatReq struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// Message OpenAI 消息
type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// LLMDelta SSE 流式响应（OpenAI 格式）
type LLMDelta struct {
	ID      string `json:"id,omitempty"`
	Object  string `json:"object,omitempty"`
	Created int64  `json:"created,omitempty"`
	Model   string `json:"model,omitempty"`
	Choices []struct {
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

// LLMClient BFF → 外部 LLM HTTP 客户端
type LLMClient interface {
	// ChatStream 流式对话（逐块回调 content）
	// ctx 取消时调用方负责停止读取（client 不负责超时管理）
	ChatStream(ctx context.Context, req LLMChatReq, onDelta func(content string)) error
}

// NewLLMClient 构造（HTTP）
func NewLLMClient(opts LLMOptions) LLMClient {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if opts.BaseURL == "" {
		opts.BaseURL = "https://api.deepseek.com"
	}
	if opts.Model == "" {
		opts.Model = "deepseek-chat"
	}
	return &llmHTTPClient{
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		apiKey:  opts.APIKey,
		model:   opts.Model,
		client: &http.Client{Timeout: timeout},
	}
}

type llmHTTPClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func (c *llmHTTPClient) ChatStream(ctx context.Context, req LLMChatReq, onDelta func(content string)) error {
	if req.Model == "" {
		req.Model = c.model
	}
	req.Stream = true
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("llm: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("llm: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Msg: fmt.Sprintf("llm: upstream %d: %s", resp.StatusCode, string(body))}
	}

	// SSE 解析：data: {json}\n\n
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		payload = strings.TrimSpace(payload)
		if payload == "" || payload == "[DONE]" {
			break
		}
		var d LLMDelta
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			continue
		}
		for _, c := range d.Choices {
			if c.Delta.Content != "" {
				onDelta(c.Delta.Content)
			}
			if c.FinishReason == "stop" {
				return nil
			}
		}
	}
	return scanner.Err()
}
