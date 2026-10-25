// Package memory — graph_writer.go
//
// CL-08c · Phase 4：图写入端（ai-svc → llm-service 的 MemoryGraphService）。
//
// 语义（ADR 决策 45 · 决策 8 / 风险 R9）：PG 是系统记录，图是可重建索引 ⇒
// 图写是 best-effort：失败只记日志，不阻塞抽取、不进重试（缺口由 CL-08d 的
// 「从 PG 重建图」工具兜底）。未装配（Noop）= CL-08a 行为零变化。
package memory

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"time"

	emotionmemorygraph "github.com/emotion-echo/shared/pkg/emotionmemorygraph"
	sharedgrpc "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Episode 一次图写入的载荷（与 PG memory_episode 同源）。
type Episode struct {
	UserID        int64
	Name          string
	Content       string
	ValidAtUnixMs int64
	SourceKind    string
	SourceID      int64
}

// EpisodeGraphWriter 图写入接口（单测注入 Fake；生产 = gRPC）。
type EpisodeGraphWriter interface {
	// UpsertEpisode 写一条情节；成功返回 graphiti episode uuid（删除定位键）。
	UpsertEpisode(ctx context.Context, ep Episode) (string, error)
	// DeleteUserData 删掉一个用户的全部图数据（CL-08d 删全部快路径）。
	DeleteUserData(ctx context.Context, userID int64) error
	// RemoveEpisode 按 uuid 摘掉一条情节（CL-08d 删单条图侧闭环）。
	RemoveEpisode(ctx context.Context, userID int64, episodeUUID string) error
}

// NoopGraphWriter 未装配 / 禁用时的空实现。
type NoopGraphWriter struct{}

func (NoopGraphWriter) UpsertEpisode(ctx context.Context, ep Episode) (string, error) {
	return "", nil
}

func (NoopGraphWriter) DeleteUserData(ctx context.Context, userID int64) error { return nil }

func (NoopGraphWriter) RemoveEpisode(ctx context.Context, userID int64, uuid string) error {
	return nil
}

// EpisodeToPB 载荷 → proto 请求（契约钉见 graph_writer_test.go）。
func EpisodeToPB(ep Episode) *emotionmemorygraph.UpsertEpisodeRequest {
	return &emotionmemorygraph.UpsertEpisodeRequest{
		UserId:        ep.UserID,
		Name:          ep.Name,
		Content:       ep.Content,
		ValidAtUnixMs: ep.ValidAtUnixMs,
		SourceKind:    ep.SourceKind,
		SourceId:      ep.SourceID,
	}
}

// GraphWriter gRPC 实现。
type GraphWriter struct {
	client emotionmemorygraph.MemoryGraphServiceClient
}

// NewGraphWriterWithAddr dial llm-service 并返回写入端。
// key 走 CL-F-66 的客户端鉴权拦截器（x-internal-api-key）；dial 失败返回 error
//（调用方降级为 Noop + warn，绝不 hard fail —— 图是增强不是依赖）。
//
// 传输层：llm-service gRPC 要求 mTLS（compose TLS_ENABLED=1，E2E-16 起）⇒
// 默认走 mTLS（与 BFF llm_grpc.go 同范式：CA 池 + 客户端证书对 + ServerName）；
// MEMORY_GRAPH_TLS=0 显式降级明文（本地裸进程调试用）。
func NewGraphWriterWithAddr(addr, apiKey string, timeout time.Duration) (*GraphWriter, error) {
	creds := insecure.NewCredentials()
	if os.Getenv("MEMORY_GRAPH_TLS") != "0" {
		ca, err := loadCert(os.Getenv("MEMORY_GRAPH_CA_CERT"), "/app/etc/tls/ca.crt")
		if err != nil {
			return nil, fmt.Errorf("graph writer ca cert: %w", err)
		}
		certPEM, err := loadCert(os.Getenv("MEMORY_GRAPH_CLIENT_CERT"), "/app/etc/tls/ai-client.crt")
		if err != nil {
			return nil, fmt.Errorf("graph writer client cert: %w", err)
		}
		keyPEM, err := loadCert(os.Getenv("MEMORY_GRAPH_CLIENT_KEY"), "/app/etc/tls/ai-client.key")
		if err != nil {
			return nil, fmt.Errorf("graph writer client key: %w", err)
		}
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("graph writer keypair: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("graph writer: append ca to pool failed")
		}
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:      pool,
			Certificates: []tls.Certificate{pair},
			ServerName:   firstNonEmpty(os.Getenv("MEMORY_GRAPH_TLS_SERVER_NAME"), "emotion-llm-service"),
			MinVersion:   tls.VersionTLS12,
		})
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds),
		grpc.WithChainUnaryInterceptor(sharedgrpc.NewClientAuthInterceptor(apiKey)),
		grpc.WithTimeout(timeout),
	)
	if err != nil {
		return nil, err
	}
	return &GraphWriter{client: emotionmemorygraph.NewMemoryGraphServiceClient(conn)}, nil
}

// loadCert 读证书文件（env 覆盖默认容器路径；单测可注入临时文件）。
func loadCert(envPath, defaultPath string) ([]byte, error) {
	p := envPath
	if p == "" {
		p = defaultPath
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UpsertEpisode 写一条情节；成功返回 graphiti 分配的 episode uuid（CL-08d：
// 落 PG memory_episode.graph_uuid，删除闭环的定位键）。
// 服务端降级（ok=false）或 RPC 错误都折算为 error。
func (w *GraphWriter) UpsertEpisode(ctx context.Context, ep Episode) (string, error) {
	resp, err := w.client.UpsertEpisode(ctx, EpisodeToPB(ep))
	if err != nil {
		return "", err
	}
	if !resp.GetOk() {
		return "", &GraphWriteRefusedError{Message: resp.GetMessage()}
	}
	return resp.GetEpisodeUuid(), nil
}

// RemoveEpisode 按 uuid 摘一条情节。
func (w *GraphWriter) RemoveEpisode(ctx context.Context, userID int64, episodeUUID string) error {
	resp, err := w.client.RemoveEpisode(ctx, &emotionmemorygraph.RemoveEpisodeRequest{
		UserId: userID, EpisodeUuid: episodeUUID,
	})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return &GraphWriteRefusedError{Message: resp.GetMessage()}
	}
	return nil
}

// DeleteUserData 删用户全部图数据（物理分图快路径）。
func (w *GraphWriter) DeleteUserData(ctx context.Context, userID int64) error {
	resp, err := w.client.DeleteUserData(ctx, &emotionmemorygraph.DeleteUserDataRequest{UserId: userID})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return &GraphWriteRefusedError{Message: resp.GetMessage()}
	}
	return nil
}

// GraphWriteRefusedError 服务端可达但拒绝/降级（ok=false）。
type GraphWriteRefusedError struct{ Message string }

func (e *GraphWriteRefusedError) Error() string { return "memory graph refused: " + e.Message }

// NewGraphWriterFromEnv 按 env 装配：MEMORY_GRAPH_ADDR 为空 ⇒ Noop；
// MEMORY_GRAPH_API_KEY 回落 AI_LLM_INTERNAL_API_KEY → INTERNAL_API_KEY（CL-F-66 范式）。
func NewGraphWriterFromEnv() EpisodeGraphWriter {
	addr := os.Getenv("MEMORY_GRAPH_ADDR")
	if addr == "" {
		return NoopGraphWriter{}
	}
	key := firstNonEmpty(os.Getenv("MEMORY_GRAPH_API_KEY"),
		os.Getenv("AI_LLM_INTERNAL_API_KEY"), os.Getenv("INTERNAL_API_KEY"))
	w, err := NewGraphWriterWithAddr(addr, key, 10*time.Second)
	if err != nil {
		slog.Warn("memory graph writer unavailable (noop)", "addr", addr, "err", err)
		return NoopGraphWriter{}
	}
	return w
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
