// Package sse — stream.go
//
// Stage 30 / stage-30-web-bff.md T3.27-28: 把分析结果编码为 SSE 事件流。
//
// 规划偏差说明：stage-30-web-bff.md 假设 ai-svc EmotionQueryService 是
// streaming gRPC（"3×delta + finish"）。实际 ai-svc 的 EmotionQueryService
// 是 **unary**（GetEmotionByMessage / GetEmotionByConversation，见
// emotion-echo-shared/pkg/emotionquery）。因此本实现把 **unary 结果**编码为
// SSE 事件序列（event: analysis → event: done），对外仍是标准 SSE 流。
// 未来 ai-svc 若加 streaming RPC，可替换本函数内部为流式消费，接口不变。
package sse

import (
	"fmt"
	"io"
)

// AnalysisResult 是 SSE 输出的情绪分析结果形状（前端 useAIStream 消费）
type AnalysisResult struct {
	MessageID      int64   `json:"messageId"`
	ConversationID int64   `json:"conversationId"`
	PrimaryEmotion string  `json:"primaryEmotion"`
	SentimentScore float64 `json:"sentimentScore"`
	Confidence     float64 `json:"confidence"`
	Model          string  `json:"model"`
}

// StreamAnalysis 把单条情绪分析结果编码为 SSE 事件流：
//   event: analysis（含完整结果）
//   event: done（{"status":"ok"}）
//
// 返回错误时调用方应中断响应（连接已半开）。
func StreamAnalysis(w io.Writer, res AnalysisResult) error {
	if err := Encode(w, Event{
		Name: "analysis",
		Data: res,
	}); err != nil {
		return fmt.Errorf("sse: write analysis event: %w", err)
	}
	if err := Encode(w, Event{
		Name: "done",
		Data: map[string]any{"status": "ok"},
	}); err != nil {
		return fmt.Errorf("sse: write done event: %w", err)
	}
	return nil
}
