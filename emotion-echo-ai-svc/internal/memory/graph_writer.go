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
	"log/slog"
	"os"
	"time"

	emotionmemorygraph "github.com/emotion-echo/shared/pkg/emotionmemorygraph"
	sharedgrpc "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"google.golang.org/grpc"
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
	UpsertEpisode(ctx context.Context, ep Episode) error
}

// NoopGraphWriter 未装配 / 禁用时的空实现。
type NoopGraphWriter struct{}

func (NoopGraphWriter) UpsertEpisode(ctx context.Context, ep Episode) error { return nil }

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
func NewGraphWriterWithAddr(addr, apiKey string, timeout time.Duration) (*GraphWriter, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(sharedgrpc.NewClientAuthInterceptor(apiKey)),
		grpc.WithTimeout(timeout),
	)
	if err != nil {
		return nil, err
	}
	return &GraphWriter{client: emotionmemorygraph.NewMemoryGraphServiceClient(conn)}, nil
}

// UpsertEpisode 写一条情节；服务端降级（ok=false）或 RPC 错误都折算为 error。
func (w *GraphWriter) UpsertEpisode(ctx context.Context, ep Episode) error {
	resp, err := w.client.UpsertEpisode(ctx, EpisodeToPB(ep))
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
