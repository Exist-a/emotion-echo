// Package fusion — Stage 34 · PR-12 GREEN
//
// LLMFuser 是 fusion 的"主路径"算法，复用 BFF 现有的 LLM_BASE_URL。
//
// 协议：OpenAI chat completions 兼容（DeepSeek / OpenAI / 任意兼容服务）。
//
// Prompt 设计：
//   - System: "你是一个情绪融合器..." (中文)
//   - User: 序列化 ModalitySnapshot 为 JSON
//   - LLM 输出 JSON: {primary_emotion, sentiment_score, modality_contrib, reasoning}
//
// 失败语义：
//   - 网络/超时/HTTP 非 2xx → error
//   - LLM 返回非 JSON → error
//   - 解析后的字段缺失（primary_emotion 为空）→ error
//   错误一律返回让 Worker 走 late_fuser 兜底。
package fusion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"emotion-echo-ai-svc/internal/model"
)

// LLMConfig LLM HTTP 客户端配置。
type LLMConfig struct {
	BaseURL string        // e.g. "https://api.deepseek.com"
	APIKey  string        // Bearer token
	Model   string        // e.g. "deepseek-chat"
	Timeout time.Duration // 单次 HTTP 调用超时
}

// LLMFuser 调真实 LLM 做情绪融合。
type LLMFuser struct {
	cfg LLMConfig
	cli *http.Client
}

// NewLLMFuser 构造器。
func NewLLMFuser(cfg LLMConfig) *LLMFuser {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &LLMFuser{
		cfg: cfg,
		cli: &http.Client{Timeout: timeout},
	}
}

// llmChatRequest OpenAI 兼容 chat completions 请求体（最小字段）。
type llmChatRequest struct {
	Model    string         `json:"model"`
	Messages []llmChatMsg   `json:"messages"`
	Response *llmJSONSchema `json:"response_format,omitempty"`
}

// llmChatMsg 单条消息。
type llmChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// llmJSONSchema OpenAI JSON mode 提示（部分兼容实现支持）。
type llmJSONSchema struct {
	Type string `json:"type"`
}

// llmChatResponse OpenAI 兼容响应（只解析 choices[0].message.content）。
type llmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// llmFusedOutput LLM 输出的 JSON 结构。
type llmFusedOutput struct {
	PrimaryEmotion  string             `json:"primary_emotion"`
	SentimentScore  float64            `json:"sentiment_score"`
	ModalityContrib map[string]float64 `json:"modality_contrib"`
	Reasoning       string             `json:"reasoning"`
}

// systemPrompt 中文固定 prompt。
const llmSystemPrompt = `你是一个多模态情绪融合器。下面会给你同一消息的多路情绪识别结果（每路含 emotion/confidence/sentiment）。请综合判断：

1. 综合情绪标签（happy/sad/angry/neutral/calm/anxious/...）
2. sentiment_score（-1 到 1）
3. 每路贡献度 modality_contrib（key 是 "text"/"voice"/"face"，value 在 0~1 之间，所有 key 之和=1）
4. 简短 reasoning（一句话）

只输出 JSON，不要其他文字。`

// Fuse 是 Fuser 接口实现。
func (f *LLMFuser) Fuse(ctx context.Context, s ModalitySnapshot) (*model.FusedEmotion, error) {
	if s.IsEmpty() {
		return nil, errors.New("llm fusion: no modalities available")
	}

	// 1. 序列化 snapshot
	snapJSON, err := json.Marshal(map[string]interface{}{
		"text":  s.Text,
		"face":  s.Face,
		"voice": s.Voice,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	// 2. 构造请求
	req := llmChatRequest{
		Model: f.cfg.Model,
		Messages: []llmChatMsg{
			{Role: "system", Content: llmSystemPrompt},
			{Role: "user", Content: string(snapJSON)},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 3. 调 LLM
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if f.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+f.cfg.APIKey)
	}

	resp, err := f.cli.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, string(buf))
	}

	// 4. 解析响应
	var chatResp llmChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal chat response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, errors.New("llm response has no choices")
	}
	content := chatResp.Choices[0].Message.Content

	// 5. 解析 LLM 输出 JSON（content 本身又是 JSON 字符串）
	//    Stage 35 PR-1：先剥 markdown 包装 + 双重 JSON 解码。
	//    真实 LLM（DeepSeek/OpenAI/Llama 兼容）即使 system prompt 要求 "只输出 JSON"
	//    仍偶发用 ```json...``` 包裹，少数情况会把 JSON 字符串再序列化一次（双重编码）。
	//    unwrapLLMContent 同时处理这两种情况，确保下游 json.Unmarshal 拿到的是干净 JSON。
	content = unwrapLLMContent(content)
	var out llmFusedOutput
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("unmarshal llm output: %w", err)
	}
	// Stage 35 PR-2：完整 schema 校验（emotion 白名单 + sentiment 范围 + modality_contrib 总和）。
	// 失败返回 error → Worker 走 late_fuser 兜底，避免污染下游。
	if err := validateLLMOutput(out); err != nil {
		return nil, fmt.Errorf("llm output validation: %w", err)
	}

	// 6. 序列化 modality_contrib 回字符串
	contribJSON, err := json.Marshal(out.ModalityContrib)
	if err != nil {
		return nil, fmt.Errorf("marshal modality_contrib: %w", err)
	}

	return &model.FusedEmotion{
		PrimaryEmotion:      out.PrimaryEmotion,
		SentimentScore:      out.SentimentScore,
		Confidence:          1.0, // LLM 输出视为高 confidence；产品可后续调
		ModalityContrib:     string(contribJSON),
		Reasoning:           out.Reasoning,
		FusionMethod:        "llm",
		AvailableModalities: model.AvailableModalitiesFromSlice(s.AvailableModalities()),
	}, nil
}
