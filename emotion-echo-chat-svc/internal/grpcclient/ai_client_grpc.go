// Package grpcclient — ai_client_grpc.go
//
// Stage 36-A3.2: ai-svc gRPC client 真实现（走 google.golang.org/grpc）。
//
// 设计：
//   - NewAIgRPCClient(aiSvcAddr) 构造并 dial ai-svc gRPC server
//   - UpsertNeutralEmotion 调 EmotionQueryService.UpsertNeutralEmotion
//   - dial 失败不 panic（NewAIgRPCClient 返回 error，main.go 决定降级为 NoopAIClient 还是 hard fail）
//   - 不带 client-side user-id metadata（chat-svc 是 producer 不是 consumer；ai-svc 那边的
//     x-user-id 拦截器对 producer RPC 不要求；Stage 32 PR-16 注释说明）
package grpcclient

import (
	"context"
	"fmt"
	"time"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// aigrpcClient 是 AIClient 的 gRPC 实现
type aigrpcClient struct {
	conn   *grpc.ClientConn
	client emotionquery.EmotionQueryServiceClient
}

// NewAIgRPCClient dial ai-svc gRPC server 并返回 AIClient。
//
// aiSvcAddr 形如 "ai-svc:8892"（容器内 DNS）或 "localhost:8892"（本地）。
// dial 超时 5s，足够慢启动的 ai-svc。
func NewAIgRPCClient(aiSvcAddr string) (AIClient, error) {
	if aiSvcAddr == "" {
		return nil, fmt.Errorf("ai-svc grpc addr is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, aiSvcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial ai-svc gRPC %s: %w", aiSvcAddr, err)
	}
	return &aigrpcClient{
		conn:   conn,
		client: emotionquery.NewEmotionQueryServiceClient(conn),
	}, nil
}

// UpsertNeutralEmotion 实现 AIClient 接口
func (c *aigrpcClient) UpsertNeutralEmotion(ctx context.Context, req *emotionquery.UpsertNeutralEmotionRequest) (*emotionquery.UpsertNeutralEmotionResponse, error) {
	return c.client.UpsertNeutralEmotion(ctx, req)
}

// Close 释放 gRPC 连接
func (c *aigrpcClient) Close() error {
	return c.conn.Close()
}
