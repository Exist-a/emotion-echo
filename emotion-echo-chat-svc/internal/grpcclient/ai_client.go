// Package grpcclient — ai_client.go
//
// Stage 36-A3.2: chat-svc 首次接入 ai-svc gRPC client（用于 Kafka 关闭时的
// dev fallback 同步写中性占位情绪）。
//
// 设计动机：Kafka 关闭（KAFKA_ENABLED=false / dev 模式）时，chat-svc 发消息
// 不会经过 Kafka → ai-svc 永远不会写入 emotion_analysis，前端"情绪分析"模块
// 在 dev 模式下完全无数据。本 client 让 chat-svc 在 SendMessage 成功后
// 同步调 ai-svc UpsertNeutralEmotion，写入中性占位。
//
// 设计要点：
//   - 接口定义在本包（不在 import emotionquery 的位置），便于 SendMessageLogic
//     注入 mock 替身（测试用 fakeAIClient）。
//   - 实现 aigrpcClient 走 google.golang.org/grpc dial ai-svc gRPC server。
//   - 空实现 noopAIClient 用于"未配 AI_GRPC_ADDR / 不需要 dev fallback"的场景，
//     避免 SendMessageLogic 每次判 nil。
package grpcclient

import (
	"context"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
)

// AIClient 是 chat-svc → ai-svc gRPC 客户端的最小接口。
//
// 只暴露 Stage 36-A3 需要的 UpsertNeutralEmotion；后续可加 GetEmotionByMessage
// 等读路径（chat-svc 暂时不需要）。
type AIClient interface {
	UpsertNeutralEmotion(ctx context.Context, req *emotionquery.UpsertNeutralEmotionRequest) (*emotionquery.UpsertNeutralEmotionResponse, error)
}

// NoopAIClient 空实现：什么都不做。给"未配 AI_GRPC_ADDR / 不需要 dev fallback"
// 的场景使用，SendMessageLogic 不需要判 nil。
type NoopAIClient struct{}

// UpsertNeutralEmotion 空实现：直接返回 nil（无错）。
func (NoopAIClient) UpsertNeutralEmotion(_ context.Context, _ *emotionquery.UpsertNeutralEmotionRequest) (*emotionquery.UpsertNeutralEmotionResponse, error) {
	return nil, nil
}
