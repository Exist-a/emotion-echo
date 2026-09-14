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
//
// Stage 94 PR-2 §P0-2：dial options 加 sharedgrpc.ClientDialOptions 链
// (tracing + timeout + logging) — 让 chat-svc → ai-svc 的 sw8 metadata
// 跨 gRPC 进程透传到 ai-svc server 端。Stage 94 PR-3 已修 NewClientTracingInterceptor
// 用 CreateExitSpan + metadata.MD 注入 sw8。本 PR 让 chat-svc 这边也接入 helper,
// 与 web-bff PR-4 / ai-svc→llm-service 同模式。retry 不挂(ai-svc 业务调用,
// single-attempt 即可,失败走 NoopAIClient 降级)。
package grpcclient

import (
	"context"
	"fmt"
	"time"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"github.com/SkyAPM/go2sky"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// traceTracer returns the global SkyWalking tracer (nil if not initialized).
// 对齐 ai-svc/internal/analyzer/grpc_analyzer.go 模式。
func traceTracer() *go2sky.Tracer {
	return skywalking.Tracer()
}

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
	// Stage 94 PR-2 §P0-2：挂 shared ClientDialOptions(tracing + 5s timeout + logging)
	// 让 chat-svc → ai-svc 的 sw8 跨 gRPC 进程透传。
	// (Stage 94 PR-3 已修 NewClientTracingInterceptor 内部走 metadata.MD 注入)
	dialOpts := grpcinterceptor.ClientDialOptions(
		grpcinterceptor.NewGo2SkyTracer(traceTracer()),
		5*time.Second,
	)
	conn, err := grpc.DialContext(ctx, aiSvcAddr,
		append([]grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		}, dialOpts...)...,
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
