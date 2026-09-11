// Package downstream — ai_grpc.go
//
// Sprint F2（2026-09-11）：BFF → ai-svc gRPC client
//
// 设计（与 chat_grpc.go / user_grpc.go 同模式）：
//   - AIClient interface 不变（ai.go 已定义 3 个方法：MultiModalAnalyze / SynthesizeSpeech / AIHealth）
//   - aiGRPCClient 实现 AIClient；通过内嵌 aiHTTPClient 仅覆盖 3 个 gRPC 方法
//   - feature flag: AIClientOptions.Transport=grpc（默认）| http
//
// proto 类型：直接复用 shared/pkg/emotionquery 生成代码（emotion_query.proto Sprint F2
// 扩 3 RPC：MultiModalAnalyze / SynthesizeSpeech / AIHealth）
//
// 鉴权：metadata x-user-id（与 emotion_query.proto / ai-svc 拦截器一致）
package downstream

import (
	"bytes"
	"context"
	"fmt"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"

	"google.golang.org/grpc"
)

// aiGRPCClient 是 AIClient 的 gRPC 实现（组合 aiHTTPClient）
type aiGRPCClient struct {
	conn        *grpc.ClientConn
	httpFallback *aiHTTPClient
}

// NewAIGRPCClient 构造（conn=nil → 返 nil，由 NewAIClient 工厂处理 fallback）
func NewAIGRPCClient(conn *grpc.ClientConn) AIClient {
	if conn == nil {
		return nil
	}
	return &aiGRPCClient{conn: conn}
}

// MultiModalAnalyze gRPC RPC
//
// proto MultiModalAnalyzeRequest.file_bytes (bytes) → logic.Analyze 第二参数
func (c *aiGRPCClient) MultiModalAnalyze(ctx context.Context, req MultiModalAnalyzeReq) (*MultiModalAnalyzeResp, error) {
	cli := emotionquery.NewEmotionQueryServiceClient(c.conn)
	grpcReq := &emotionquery.MultiModalAnalyzeRequest{
		Kind:        req.Kind,
		Filename:    req.FileName,
		FileBytes:   fileToBytes(req.File),
		TextContent: req.Text,
		// persist / upload_id / message_id / user_id / conversation_id / transcript
		// / duration_ms / language：当前 BFF 调用方未传（chat-svc 不经 BFF 直接调 ai-svc），
		// 暂不映射。如未来 chat-svc 经 BFF 触发，再加字段映射。
	}
	resp, err := cli.MultiModalAnalyze(withUserID(ctx), grpcReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: ai multi modal analyze: %w", err)
	}
	return fromProtoMultiModalAnalyze(resp), nil
}

// SynthesizeSpeech gRPC RPC
func (c *aiGRPCClient) SynthesizeSpeech(ctx context.Context, req SynthesizeSpeechReq) (*SynthesizeSpeechResp, error) {
	cli := emotionquery.NewEmotionQueryServiceClient(c.conn)
	resp, err := cli.SynthesizeSpeech(withUserID(ctx), &emotionquery.SynthesizeSpeechRequest{
		Text:     req.Text,
		Language: req.Language,
		Speed:    req.Speed,
	})
	if err != nil {
		return nil, fmt.Errorf("downstream: ai synthesize speech: %w", err)
	}
	return fromProtoSynthesizeSpeech(resp), nil
}

// AIHealth gRPC RPC（鉴权需要 x-user-id）
func (c *aiGRPCClient) AIHealth(ctx context.Context) (*AIHealthResp, error) {
	cli := emotionquery.NewEmotionQueryServiceClient(c.conn)
	resp, err := cli.AIHealth(withUserID(ctx), &emotionquery.AIHealthRequest{})
	if err != nil {
		return nil, fmt.Errorf("downstream: ai health: %w", err)
	}
	return fromProtoAIHealth(resp), nil
}

// ============ proto → types 转换 ============

func fromProtoMultiModalAnalyze(r *emotionquery.MultiModalAnalyzeResponse) *MultiModalAnalyzeResp {
	if r == nil {
		return nil
	}
	return &MultiModalAnalyzeResp{
		Kind:       r.GetKind(),
		Emotion:    r.GetEmotion(),
		Confidence: r.GetConfidence(),
		// proto SentimentScore (double) → types Sentiment (float64)：字段名映射
		SentimentScore: r.GetSentimentScore(),
		Model:      r.GetModel(),
		Transcript: r.GetTranscript(),
		AllScores:  r.GetAllScores(),
	}
}

func fromProtoSynthesizeSpeech(r *emotionquery.SynthesizeSpeechResponse) *SynthesizeSpeechResp {
	if r == nil {
		return nil
	}
	return &SynthesizeSpeechResp{
		Audio:      r.GetAudio(),
		SampleRate: int(r.GetSampleRate()),
		Mime:       r.GetMime(),
		Bytes:      int(r.GetBytes()),
		Text:       r.GetText(),
		Language:   r.GetLanguage(),
	}
}

func fromProtoAIHealth(r *emotionquery.AIHealthResponse) *AIHealthResp {
	if r == nil {
		return nil
	}
	out := &AIHealthResp{
		Time:       r.GetTimeMs(),
		AllHealthy: r.GetAllHealthy(),
	}
	if e := r.GetFer(); e != nil {
		out.FER = &AIHealthEntry{
			Enabled: e.GetEnabled(),
			Healthy: e.GetHealthy(),
			Error:   e.GetError(),
		}
	}
	if e := r.GetSensevoice(); e != nil {
		out.SenseVoice = &AIHealthEntry{
			Enabled: e.GetEnabled(),
			Healthy: e.GetHealthy(),
			Error:   e.GetError(),
		}
	}
	if e := r.GetXtts(); e != nil {
		out.XTTS = &AIHealthEntry{
			Enabled: e.GetEnabled(),
			Healthy: e.GetHealthy(),
			Error:   e.GetError(),
		}
	}
	return out
}

// fileToBytes 把 io.Reader 接口（multipart File）读到 []byte
//
// gRPC 不能传流式 io.Reader，必须一次性 bytes。生产环境大文件可能占用内存，
// 但 ai-svc 本次 Sprint F2 也不支持 stream（proto field bytes）。生产用大文件时
// 应考虑 chunked upload（Sprint F3+）。
func fileToBytes(r interface{ Read(p []byte) (n int, err error) }) []byte {
	if r == nil {
		return nil
	}
	buf := &bytes.Buffer{}
	if _, err := buf.ReadFrom(r); err != nil {
		// 读失败返 nil（与 nil file 同语义）
		return nil
	}
	return buf.Bytes()
}
