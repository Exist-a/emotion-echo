// Package analyzer 的 gRPC 实现
//
// GRPCAnalyzer：通过 gRPC 调用 emotion-llm-service
// 对应 HTTPAnalyzer（旧实现），保留以作 fallback/对比
//
// 设计动机：
//   - HTTP 是文本协议，每次调用要重新解析 JSON（CPU 开销）
//   - gRPC 用 protobuf 二进制 + HTTP/2，序列化快 5-10 倍
//   - 强契约（.proto 文件是 SSoT）
//   - 支持 streaming / cancellation
package analyzer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"emotion-echo-ai-svc/internal/logging"

	emotionllm "github.com/emotion-echo/shared/pkg/emotionllm"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/healthcheck"
	"github.com/emotion-echo/shared/pkg/skywalking"

	"github.com/SkyAPM/go2sky"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// traceTracer returns the global SkyWalking tracer (nil if not initialized).
// Used by NewClientTracingInterceptor; safe to call before skywalking.Init().
func traceTracer() *go2sky.Tracer {
	return skywalking.Tracer()
}

// GRPCAnalyzer 通过 gRPC 调用 LLM 服务
//
// 协议：emotion_llm.proto 中定义的 EmotionLLMService.Analyze
// 优势：protobuf 序列化 + HTTP/2 多路复用
type GRPCAnalyzer struct {
	conn   *grpc.ClientConn
	client emotionllm.EmotionLLMServiceClient
}

// NewGRPCAnalyzer constructs a gRPC analyzer.
//
// target: gRPC server address, e.g. "localhost:50051".
//
// Interceptors (Stage 11):
//   - ClientLogging: logs every RPC (method, target, latency, err)
//   - ClientTimeout(3s): per-call timeout to avoid blocking consumer
//
// Stage 14 升级：dial 后调用 grpc.health.v1 WaitForReady，
// 确保下游 LLM 服务的"业务就绪"（不仅是 TCP 连通）。
//
// Stage 18 升级：支持 mTLS（通过 TLS_ENABLED=1 启用，从 env 读证书路径）
func NewGRPCAnalyzer(target string) (*GRPCAnalyzer, error) {
	creds, err := buildClientCredentials()
	if err != nil {
		return nil, fmt.Errorf("build client credentials: %w", err)
	}

	// 非阻塞 dial：先建立 TCP，再做业务级 health check
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(creds),
		grpc.WithChainUnaryInterceptor(
			grpcinterceptor.NewClientTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(traceTracer())),
			grpcinterceptor.ClientLoggingInterceptor(),
			grpcinterceptor.ClientTimeoutInterceptor(3*time.Second),
			// Stage 15：transient 错误自动重试
			grpcinterceptor.ClientRetryInterceptor(grpcinterceptor.DefaultRetryOptions()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc new client %s failed: %w", target, err)
	}

	// Stage 14：业务级 health check（gRPC 标准 health/v1 协议）
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer healthCancel()
	healthCli := healthcheck.NewClient(conn)
	if err := healthCli.WaitForReady(healthCtx, "emotion.LLM", 5*time.Second); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("grpc health check %s/emotion.LLM failed: %w", target, err)
	}
	logging.Printf("[grpc-health] target=%s service=emotion.LLM status=SERVING", target)

	return &GRPCAnalyzer{
		conn:   conn,
		client: emotionllm.NewEmotionLLMServiceClient(conn),
	}, nil
}

// Close 关闭 gRPC 连接
func (a *GRPCAnalyzer) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// Analyze 通过 gRPC 调远程 LLM 服务分析文本
//
// 错误：网络错误 / gRPC 状态码非 OK / 响应字段缺失
func (a *GRPCAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	resp, err := a.client.Analyze(ctx, &emotionllm.AnalyzeRequest{
		Text: text,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc analyze failed: %w", err)
	}

	return &EmotionResult{
		PrimaryEmotion: resp.GetPrimaryEmotion(),
		SentimentScore: resp.GetSentimentScore(),
		Confidence:     resp.GetConfidence(),
		Model:          resp.GetModel(),
	}, nil
}

// buildClientCredentials 根据环境变量选择 insecure 或 mTLS credentials
//
// 配置：
//   - TLS_ENABLED=1 → 启用 mTLS（默认 cert 路径：deploy/tls/*.crt）
//   - TLS_CA_CERT / TLS_CLIENT_CERT / TLS_CLIENT_KEY 可覆盖路径
//   - TLS_SERVER_NAME 校验 server cert CN（默认 emotion-llm-service）
func buildClientCredentials() (credentials.TransportCredentials, error) {
	tlsEnabled := os.Getenv("TLS_ENABLED") == "1"
	if !tlsEnabled {
		return insecure.NewCredentials(), nil
	}

	caPath := getEnvOrDefault("TLS_CA_CERT", "deploy/tls/ca.crt")
	certPath := getEnvOrDefault("TLS_CLIENT_CERT", "deploy/tls/ai-client.crt")
	keyPath := getEnvOrDefault("TLS_CLIENT_KEY", "deploy/tls/ai-client.key")
	serverName := getEnvOrDefault("TLS_SERVER_NAME", "emotion-llm-service")

	// mTLS：client 端提供自己的 cert/key 让 server 验证身份
	creds, err := credentials.NewClientTLSFromFile(caPath, serverName)
	if err != nil {
		return nil, fmt.Errorf("NewClientTLSFromFile (ca): %w", err)
	}
	// 用 NewTLS 包装让 client 端也发自己的 cert（双向认证）
	tlsCfg := creds.Info().SecurityProtocol
	_ = tlsCfg

	// 用 mtls 模式：先 NewClientTLSFromFile 拿到 ServerName / RootCAs 模板，
	// 再注入 ClientCAs / ClientAuth = RequireAndVerifyClientCert
	pool := x509.NewCertPool()
	caData, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read ca: %w", err)
	}
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("append ca to pool failed")
	}
	clientCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      pool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}
	logging.Printf("[grpc-tls] mTLS client enabled: ca=%s cert=%s server_name=%s", caPath, certPath, serverName)
	return credentials.NewTLS(tlsConfig), nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
//
// Stage 12 升级：internal svc-to-svc 调用走 API key 鉴权。
// ai-svc 启动时从 config.LLM.InternalAPIKey 读取 key。
func (a *GRPCAnalyzer) AnalyzeWithAuth(ctx context.Context, text, apiKey string) (*EmotionResult, error) {
	authCtx := grpcinterceptor.WithInternalAPIKey(ctx, apiKey)
	return a.Analyze(authCtx, text)
}