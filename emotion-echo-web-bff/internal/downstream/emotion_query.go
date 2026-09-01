// Package downstream — emotion_query.go
//
// Stage 30 / stage-30-web-bff.md T2.24: EmotionQueryClient（BFF → ai-svc gRPC）
//
// ai-svc gRPC :8892，service emotion_ai.v1.EmotionQueryService（unary）：
//   GetEmotionByMessage(message_id) → Emotion
//   GetEmotionByConversation(conversation_id, limit) → EmotionList{items, total}
//
// 直接复用 shared 生成的 emotionquery 类型（github.com/emotion-echo/shared/pkg/emotionquery），
// 不重复定义 proto 模型。svc-to-svc gRPC 无鉴权（server 侧无 auth interceptor）。
package downstream

import (
	"context"
	"fmt"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"google.golang.org/grpc"
)

// EmotionQueryClient BFF → ai-svc EmotionQueryService gRPC 客户端
type EmotionQueryClient interface {
	// ByMessage 查单条消息的情绪
	ByMessage(ctx context.Context, messageID int64) (*emotionquery.Emotion, error)
	// ByConversation 查会话的情绪列表
	ByConversation(ctx context.Context, conversationID int64, limit int) ([]*emotionquery.Emotion, int32, error)
	// ByFusedMessage 查单条消息的多模态融合产物（Stage 34）
	ByFusedMessage(ctx context.Context, messageID int64) (*emotionquery.FusedEmotion, error)
}

// emotionQueryClient 是 EmotionQueryClient 的 gRPC 实现
type emotionQueryClient struct {
	conn *grpc.ClientConn
}

// NewEmotionQueryClient 构造（需已建立的 gRPC 连接）
func NewEmotionQueryClient(conn *grpc.ClientConn) EmotionQueryClient {
	return &emotionQueryClient{conn: conn}
}

func (c *emotionQueryClient) ByMessage(ctx context.Context, messageID int64) (*emotionquery.Emotion, error) {
	cli := emotionquery.NewEmotionQueryServiceClient(c.conn)
	resp, err := cli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: messageID})
	if err != nil {
		return nil, fmt.Errorf("downstream: emotion query by message: %w", err)
	}
	return resp, nil
}

func (c *emotionQueryClient) ByConversation(ctx context.Context, conversationID int64, limit int) ([]*emotionquery.Emotion, int32, error) {
	cli := emotionquery.NewEmotionQueryServiceClient(c.conn)
	resp, err := cli.GetEmotionByConversation(ctx, &emotionquery.GetEmotionByConversationRequest{
		ConversationId: conversationID,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("downstream: emotion query by conversation: %w", err)
	}
	return resp.Items, resp.Total, nil
}

// ByFusedMessage 调 GetFusedEmotion RPC（Stage 34）。
//
// 设计：
//   - NotFound 由 caller 处理（handler 返 404）
//   - 其他 gRPC 错误透传（caller 返 500）
func (c *emotionQueryClient) ByFusedMessage(ctx context.Context, messageID int64) (*emotionquery.FusedEmotion, error) {
	cli := emotionquery.NewEmotionQueryServiceClient(c.conn)
	resp, err := cli.GetFusedEmotion(ctx, &emotionquery.GetFusedEmotionRequest{MessageId: messageID})
	if err != nil {
		return nil, fmt.Errorf("downstream: emotion query fused by message: %w", err)
	}
	return resp, nil
}
