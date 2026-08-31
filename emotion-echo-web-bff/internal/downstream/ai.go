// Package downstream — ai.go
//
// Stage 30 / stage-30-web-bff.md T2.6-8: AIClient（BFF → ai-svc）
// Stage 32 PR-16: 鉴权透传改用 X-User-Id header（替换原 Authorization Bearer JWT）。
//
// ai-svc 双协议：
//   HTTP :8891  — MultiModalAnalyze (multipart) / SynthesizeSpeech (JSON) / AIHealth
//   gRPC :8892  — EmotionQueryService（T2.24 EmotionQueryClient 单独做）
//
// 鉴权：下游认 X-User-Id header（APISIX jwt-auth 注入，BFF 透传）。
// client 从 ctx 读 user_id（WithUserID 注入），无 user_id 时仍发请求
// （下游返回 401，由调用方决定错误语义）。
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

// =====================================================
// User ID 透传（Stage 32 PR-16: 替换原 WithJWT/JWTFromContext）
// =====================================================

type userIDCtxKey struct{}

// XUserIDHeader HTTP header 名（与 shared middleware 同源）
const XUserIDHeader = "X-User-Id"

// WithUserID 把 user_id 存入 ctx（handler 层通过 shared GinAuthMiddleware 注入，
// 或显式调用 session.WithRequestAuth 包装）。下游 client 通过 UserIDFromContext 读取。
func WithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, userIDCtxKey{}, uid)
}

// UserIDFromContext 从 ctx 取 user_id；无则返回 0, false
func UserIDFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(userIDCtxKey{}).(int64)
	return v, ok
}

// applyAuthHeader 从 ctx 读 user_id 并注入 X-User-Id header（Stage 32 PR-16）
func applyAuthHeader(req *http.Request, ctx context.Context) {
	if uid, ok := UserIDFromContext(ctx); ok && uid > 0 {
		req.Header.Set(XUserIDHeader, strconv.FormatInt(uid, 10))
	}
}

// =====================================================
// 请求/响应模型（与 ai-svc types 对齐）
// =====================================================

// MultiModalAnalyzeReq 多模态分析请求（kind=text 时无需 File）
type MultiModalAnalyzeReq struct {
	Kind     string    // text | image | audio
	File     io.Reader // kind != text 时必填
	FileName string    // 上传文件名（用于 multipart filename）
	Text     string    // 可选
}

// MultiModalAnalyzeResp 对应 ai-svc logic.MultiModalAnalyzeResp
type MultiModalAnalyzeResp struct {
	Kind           string             `json:"kind"`
	Emotion        string             `json:"emotion"`
	Confidence     float64            `json:"confidence"`
	SentimentScore float64            `json:"sentimentScore"`
	Model          string             `json:"model"`
	Transcript     string             `json:"transcript,omitempty"`
	AllScores      map[string]float64 `json:"allScores,omitempty"`
}

// SynthesizeSpeechReq 对应 ai-svc /api/v1/tts/synthesize 请求
type SynthesizeSpeechReq struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
}

// SynthesizeSpeechResp 对应 ai-svc logic.SynthesizeSpeechResp（base64 WAV）
type SynthesizeSpeechResp struct {
	Audio      string `json:"audio"`
	SampleRate int    `json:"sampleRate"`
	Mime       string `json:"mime"`
	Bytes      int    `json:"bytes"`
	Text       string `json:"text"`
	Language   string `json:"language"`
}

// AIHealthEntry 对应 ai-svc logic.AIHealthEntry
type AIHealthEntry struct {
	Enabled   bool   `json:"enabled"`
	Healthy   bool   `json:"healthy"`
	Error     string `json:"error,omitempty"`
	URL       string `json:"url,omitempty"`
	LatencyMs string `json:"latencyMs,omitempty"`
}

// AIHealthResp 对应 ai-svc logic.AIHealthResp
type AIHealthResp struct {
	Time       int64         `json:"time"`
	AllHealthy bool          `json:"allHealthy"`
	FER        *AIHealthEntry `json:"fer,omitempty"`
	SenseVoice *AIHealthEntry `json:"sensevoice,omitempty"`
	XTTS       *AIHealthEntry `json:"xtts,omitempty"`
}

// =====================================================
// 接口
// =====================================================

// AIClient BFF → ai-svc HTTP 客户端
type AIClient interface {
	// MultiModalAnalyze multipart/form-data 上传 → 情绪分析结果
	MultiModalAnalyze(ctx context.Context, req MultiModalAnalyzeReq) (*MultiModalAnalyzeResp, error)
	// SynthesizeSpeech 文本转语音（base64 WAV）
	SynthesizeSpeech(ctx context.Context, req SynthesizeSpeechReq) (*SynthesizeSpeechResp, error)
	// AIHealth 聚合 FER / SenseVoice / XTTS 健康状态
	AIHealth(ctx context.Context) (*AIHealthResp, error)
}

// =====================================================
// HTTP 实现
// =====================================================

// AIClientOptions 构造选项
type AIClientOptions struct {
	BaseURL   string
	TimeoutMs int
}

// aiHTTPClient 是 AIClient 的 HTTP 实现
type aiHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewAIClient 构造 AIClient（HTTP 实现）
func NewAIClient(opts AIClientOptions) AIClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &aiHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *aiHTTPClient) MultiModalAnalyze(ctx context.Context, req MultiModalAnalyzeReq) (*MultiModalAnalyzeResp, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("kind", req.Kind); err != nil {
		return nil, fmt.Errorf("downstream: write kind: %w", err)
	}
	if req.Text != "" {
		if err := writer.WriteField("text", req.Text); err != nil {
			return nil, fmt.Errorf("downstream: write text: %w", err)
		}
	}
	if req.File != nil {
		fw, err := writer.CreateFormFile("file", req.FileName)
		if err != nil {
			return nil, fmt.Errorf("downstream: create form file: %w", err)
		}
		if _, err := io.Copy(fw, req.File); err != nil {
			return nil, fmt.Errorf("downstream: copy file: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("downstream: close writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/multimodal/analyze", body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: multimodal analyze: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out MultiModalAnalyzeResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode multimodal resp: %w", err)
	}
	return &out, nil
}

func (c *aiHTTPClient) SynthesizeSpeech(ctx context.Context, req SynthesizeSpeechReq) (*SynthesizeSpeechResp, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal tts req: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/tts/synthesize", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: tts synthesize: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out SynthesizeSpeechResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode tts resp: %w", err)
	}
	return &out, nil
}

func (c *aiHTTPClient) AIHealth(ctx context.Context) (*AIHealthResp, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/ai/health", nil)
	if err != nil {
		return nil, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: ai health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out AIHealthResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode ai health resp: %w", err)
	}
	return &out, nil
}

// readError 从错误响应体提取 error 消息（Go svc 统一 {"error": "..."}）
func readError(resp *http.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	msg := body.Error
	if msg == "" {
		msg = fmt.Sprintf("unexpected status %d", resp.StatusCode)
	}
	return &APIError{StatusCode: resp.StatusCode, Msg: fmt.Sprintf("downstream: %s %s: %s", resp.Request.Method, resp.Request.URL.Path, msg)}
}
