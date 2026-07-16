// Package analyzer 的 HTTP 实现
//
// HTTPAnalyzer：调用外部 LLM 服务（如 emotion-llm-service）
// ChainedAnalyzer：先尝试主 analyzer（LLM），失败时回退到次 analyzer（keyword）
//
// 设计动机：
//   - LLM 服务可能宕机 / 超时 → 必须有 fallback
//   - ChainedAnalyzer 让 business logic 无感："给我一个 Analyzer，结果是情绪"
//   - 测试用 httptest mock，无需启动 Python 服务即可跑单测
package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"emotion-echo-ai-svc/internal/logging"
)

// HTTPAnalyzer 通过 HTTP 调用 LLM 服务
//
// 协议：POST {llmURL}/analyze with {"text": "..."}
// 响应：{"primaryEmotion": "...", "sentimentScore": ..., "confidence": ..., "model": "..."}
type HTTPAnalyzer struct {
	baseURL string
	client  *http.Client
}

// llmRequest 是发给 LLM 服务的请求体
type llmRequest struct {
	Text string `json:"text"`
}

// llmResponse 是从 LLM 服务接收的响应
type llmResponse struct {
	PrimaryEmotion string  `json:"primaryEmotion"`
	SentimentScore float64 `json:"sentimentScore"`
	Confidence     float64 `json:"confidence"`
	Model          string  `json:"model"`
}

// NewHTTPAnalyzer 构造 HTTP analyzer
//
// baseURL：LLM 服务根地址，如 "http://localhost:8000"
func NewHTTPAnalyzer(baseURL string) *HTTPAnalyzer {
	return &HTTPAnalyzer{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 3 * time.Second, // 短超时：LLM 挂了不能阻塞消费
		},
	}
}

// Analyze 调远程 LLM 服务分析文本
//
// 错误：网络错误 / HTTP 非 2xx / 响应解析失败
func (a *HTTPAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	body, err := json.Marshal(llmRequest{Text: text})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	url := a.baseURL + "/analyze"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(body))
	}

	var r llmResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode llm response: %w", err)
	}

	return &EmotionResult{
		PrimaryEmotion: r.PrimaryEmotion,
		SentimentScore: r.SentimentScore,
		Confidence:     r.Confidence,
		Model:          r.Model,
	}, nil
}

// =====================================================
// ChainedAnalyzer：主 + fallback
// =====================================================

// ChainedAnalyzer 先用 primary 分析；失败则 fallback 到 secondary
//
// 典型用法：primary=LLM（生产高质量）, secondary=keyword（兜底）
type ChainedAnalyzer struct {
	primary   Analyzer
	secondary Analyzer
}

// NewChainedAnalyzer 构造链式 analyzer
func NewChainedAnalyzer(primary, secondary Analyzer) *ChainedAnalyzer {
	return &ChainedAnalyzer{primary: primary, secondary: secondary}
}

// Analyze 先 primary 失败则 secondary
//
// 返回的 EmotionResult.Model 字段标识实际用了哪个 analyzer（"llm-v1" / "keyword-stub-v1"）
func (c *ChainedAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	if r, err := c.primary.Analyze(ctx, text); err == nil && r != nil {
		return r, nil
	} else {
		logging.Errorf(err, "[analyzer] primary failed, fallback to secondary")
	}
	if c.secondary == nil {
		return nil, fmt.Errorf("primary failed and no fallback")
	}
	return c.secondary.Analyze(ctx, text)
}