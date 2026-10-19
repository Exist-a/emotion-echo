// Package memory — llm_extractor.go
//
// CL-08a · A2：OpenAI 兼容端点的事实抽取器。
//
// 配置纪律（沿用决策 40 / adr-2026-10-tts-api-cosyvoice2 的同一条）：
// base_url / model / key 一律由配置注入，**不在业务代码里硬编码供应商域名或模型名**。
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"emotion-echo-ai-svc/internal/model"
)

// ErrNotConfigured 未配置 LLM base URL（抽取应被跳过而不是报错重试）。
var ErrNotConfigured = errors.New("memory: LLM base URL not configured")

// maxFactsPerMessage 单条消息最多接纳的事实数 —— 控制成本与单点污染面。
const maxFactsPerMessage = 10

// LLMConfig 抽取器配置。
type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// LLMExtractor 通过 OpenAI 兼容的 /chat/completions 抽事实。
type LLMExtractor struct {
	cfg    LLMConfig
	client *http.Client
}

func NewLLMExtractor(cfg LLMConfig) *LLMExtractor {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &LLMExtractor{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

// extractSystemPrompt 抽取指令。
//
// 精确率优先：宁可漏抽，也不要把寒暄/一次性情绪/对 AI 的指令写成"事实"。
const extractSystemPrompt = `你是一个记忆抽取器。从用户消息中抽取**关于用户本人、且长期有效**的事实。

只抽取这些类别：
- 身份 / 职业 / 教育
- 长期偏好（喜欢或讨厌的事物）
- 重要关系（家人、伴侣、朋友、宠物）
- 长期状态（居住地、健康状况、目标）
- 会长期影响用户的重要经历

不要抽取：寒暄、一次性情绪、临时计划、对 AI 的指令、问题本身。
不要推测；只写用户明确说过的内容。

只输出严格 JSON（不要 markdown 代码块，不要解释）：
{"facts":[{"subject":"用户","predicate":"职业","object":"后端工程师","text":"用户是一名后端工程师","valid_from":"2026-10-19T10:00:00Z","confidence":0.9}]}

规则：
- subject 默认写"用户"
- predicate 用简短中文名词（职业 / 居住地 / 喜欢的事物 / 家庭成员 …）
- object 用简短中文值
- text 是一句可读的完整陈述（会直接进 prompt）
- valid_from 只在用户明确给出时间时填写（RFC3339），否则省略该字段
- 没有可抽取的事实时返回 {"facts":[]}`

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type factDTO struct {
	Subject    string   `json:"subject"`
	Predicate  string   `json:"predicate"`
	Object     string   `json:"object"`
	Text       string   `json:"text"`
	ValidFrom  string   `json:"valid_from"`
	Confidence *float64 `json:"confidence"`
}

type factsEnvelope struct {
	Facts []factDTO `json:"facts"`
}

func (e *LLMExtractor) Extract(ctx context.Context, in ExtractInput) ([]model.MemoryFact, error) {
	if strings.TrimSpace(e.cfg.BaseURL) == "" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, nil
	}

	body, err := json.Marshal(chatRequest{
		Model: e.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: extractSystemPrompt},
			{Role: "user", Content: in.Content},
		},
		Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("memory: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(e.cfg.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("memory: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memory: call llm: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("memory: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("memory: llm status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, fmt.Errorf("memory: unmarshal chat response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return nil, errors.New("memory: llm returned no choices")
	}

	facts, err := parseFactsContent(cr.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	return toMemoryFacts(facts, in), nil
}

// parseFactsContent 从模型输出里解析 facts（容忍 ```json 代码块与前后杂文本）。
func parseFactsContent(content string) ([]factDTO, error) {
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("memory: no JSON object in model output: %s", truncate(content, 200))
	}
	var env factsEnvelope
	if err := json.Unmarshal([]byte(s[start:end+1]), &env); err != nil {
		return nil, fmt.Errorf("memory: unmarshal facts: %w", err)
	}
	return env.Facts, nil
}

// toMemoryFacts DTO → 领域模型；空三元组丢弃、valid_from 回落消息时间。
func toMemoryFacts(dtos []factDTO, in ExtractInput) []model.MemoryFact {
	out := make([]model.MemoryFact, 0, len(dtos))
	for _, d := range dtos {
		if len(out) >= maxFactsPerMessage {
			break
		}
		subject := strings.TrimSpace(d.Subject)
		predicate := strings.TrimSpace(d.Predicate)
		object := strings.TrimSpace(d.Object)
		if subject == "" || predicate == "" || object == "" {
			continue
		}
		text := strings.TrimSpace(d.Text)
		if text == "" {
			text = subject + predicate + object
		}
		validFrom := in.OccurredAt
		if v := strings.TrimSpace(d.ValidFrom); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				validFrom = t.UTC()
			}
		}
		if validFrom.IsZero() {
			validFrom = time.Now().UTC()
		}
		out = append(out, model.MemoryFact{
			UserID:          in.UserID,
			ConversationID:  in.ConversationID,
			SourceKind:      sourceKind(in),
			SourceID:        sourceID(in),
			Subject:         subject,
			Predicate:       predicate,
			Object:          object,
			FactText:        text,
			Confidence:      d.Confidence,
			ValidFrom:       validFrom,
			Status:          model.MemoryFactStatusActive,
		})
	}
	return out
}

// sourceKind / sourceID 归一化来源身份（空 kind 视为 message，空 id 回落 MessageID）。
func sourceKind(in ExtractInput) string {
	if in.SourceKind == "" {
		return model.MemorySourceKindMessage
	}
	return in.SourceKind
}

func sourceID(in ExtractInput) int64 {
	if in.SourceID != 0 {
		return in.SourceID
	}
	return in.MessageID
}

// chatCompletionsURL 拼接 OpenAI 兼容端点的 /chat/completions。
//
// 两种常见 base 形态都要吃：
//   - "https://api.deepseek.com"      → ".../v1/chat/completions"
//   - "https://api.openai.com/v1"     → ".../v1/chat/completions"
func chatCompletionsURL(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(b, "/v1") {
		return b + "/chat/completions"
	}
	return b + "/v1/chat/completions"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
