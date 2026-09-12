// Package downstream — llm_grpc.go
//
// Stage 81（llm-chat-real-pipeline PR-2）：BFF → llm-service ChatCompletion gRPC 客户端。
// 把 BFF 的 LLM 调用收敛到 llm-service（统一 key 管理 / 降级 / 指标 / 后续意图分类挂载点）。
//
// 连接约定（与 ai-svc GRPCAnalyzer 同款，Stage 18/17）：
//   - TLS_ENABLED=1 → mTLS（TLS_CA_CERT / TLS_CLIENT_CERT / TLS_CLIENT_KEY，
//     TLS_SERVER_NAME 默认 emotion-llm-service）
//   - INTERNAL_API_KEY 非空 → 注入 x-internal-api-key metadata
//
// 上游自身的无 key / 失败降级在 llm-service 侧完成（PR-1），本客户端不复制该逻辑：
// fallback 以 chunk.fallback_reason 透传给调用方。
package downstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	emotionllm "github.com/emotion-echo/shared/pkg/emotionllm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// LLMStreamRequest 一次对话请求（handler 侧形状）
type LLMStreamRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	MaxTokens   int32
}

// LLMChatStreamer 对话流式上游接口（handler 依赖此接口，便于测试与后续替换）
// LLMIntentClassifier 意图分类接口（LLMGRPCClient 实现；nil = 未装配）
type LLMIntentClassifier interface {
	ClassifyIntent(ctx context.Context, text string) (string, error)
}

type LLMChatStreamer interface {
	// StreamChat 阻塞式拉取整条流；每个增量 delta 调一次 onDelta(delta, model)。
	// ctx 取消（客户端断开）须中断上游流。返回 error = 传输层/上游 gRPC 状态错误
	// （chunk 级 fallback 不算 error）。
	StreamChat(ctx context.Context, req LLMStreamRequest, onDelta func(delta, model string)) error
}

// LLMGRPCOptions NewLLMGRPCClient 配置
type LLMGRPCOptions struct {
	Addr             string        // 形如 emotion-llm-service:50051
	Timeout          time.Duration // 整条流的最长等待（0 = 不限时）
	TLSEnabled       bool
	CACertPath       string
	ClientCertPath   string
	ClientKeyPath    string
	TLSServerName    string
	InternalAPIKey   string
	DialTimeoutSecs  int
}

// LLMGRPCClient BFF → llm-service gRPC 客户端
type LLMGRPCClient struct {
	conn   *grpc.ClientConn
	client emotionllm.EmotionLLMServiceClient
	apiKey string
}

// NewLLMGRPCClient 拨号 llm-service（env 驱动的 TLS/auth 由调用方从 config/env 填入 opts）
func NewLLMGRPCClient(opts LLMGRPCOptions) (*LLMGRPCClient, error) {
	if opts.Addr == "" {
		return nil, fmt.Errorf("llm grpc addr is empty")
	}

	var creds credentials.TransportCredentials
	if opts.TLSEnabled {
		caPEM, err := os.ReadFile(opts.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read ca cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("append ca to pool failed")
		}
		certPEM, keyPEM, err := tlsLoadKeyPair(opts.ClientCertPath, opts.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		clientPair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse client keypair: %w", err)
		}
		serverName := opts.TLSServerName
		if serverName == "" {
			serverName = "emotion-llm-service"
		}
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:      pool,
			Certificates: []tls.Certificate{clientPair},
			ServerName:   serverName,
			MinVersion:   tls.VersionTLS12,
		})
	} else {
		creds = insecure.NewCredentials()
	}

	// grpc.NewClient 是惰性拨号（首次 RPC 才真正建连）；llm-service 未注册 Nacos、
	// BFF 用 env 地址，配置错误会在首次请求 fail-fast 并走 handler 的降级链。
	conn, err := grpc.NewClient(opts.Addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpc NewClient(%s): %w", opts.Addr, err)
	}

	return &LLMGRPCClient{
		conn:   conn,
		client: emotionllm.NewEmotionLLMServiceClient(conn),
		apiKey: opts.InternalAPIKey,
	}, nil
}

// StreamChat 实现 LLMChatStreamer
func (c *LLMGRPCClient) StreamChat(ctx context.Context, req LLMStreamRequest, onDelta func(delta, model string)) error {
	pbMessages := make([]*emotionllm.ChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		pbMessages = append(pbMessages, &emotionllm.ChatMessage{Role: m.Role, Content: m.Content})
	}

	if c.apiKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-api-key", c.apiKey)
	}

	stream, err := c.client.ChatCompletion(ctx, &emotionllm.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    pbMessages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})
	if err != nil {
		return fmt.Errorf("llm chat completion rpc: %w", err)
	}
	for {
		chunk, err := stream.Recv()
		if err != nil {
			// io.EOF 不会出现（done 帧 + 服务端收尾），统一按错误透传
			return fmt.Errorf("llm chat stream recv: %w", err)
		}
		if chunk.GetDeltaContent() != "" {
			onDelta(chunk.GetDeltaContent(), chunk.GetModel())
		}
		if chunk.GetDone() {
			return nil
		}
	}
}

// ClassifyIntent 意图分类（Stage 82 PR-3b）：规则式 6 类，llm-service 侧实现
func (c *LLMGRPCClient) ClassifyIntent(ctx context.Context, text string) (string, error) {
	callCtx := ctx
	if c.apiKey != "" {
		callCtx = metadata.AppendToOutgoingContext(callCtx, "x-internal-api-key", c.apiKey)
	}
	resp, err := c.client.ClassifyIntent(callCtx, &emotionllm.ClassifyIntentRequest{Text: text})
	if err != nil {
		return "", fmt.Errorf("llm classify intent rpc: %w", err)
	}
	return resp.GetIntent(), nil
}

// Close 释放连接
func (c *LLMGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// tlsLoadKeyPair / tlsClientConfig 拆出小函数便于单测（避免直接依赖 crypto/tls 大段内联）
func tlsLoadKeyPair(certPath, keyPath string) (certPEM, keyPEM []byte, err error) {
	certPEM, err = os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err = os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}
