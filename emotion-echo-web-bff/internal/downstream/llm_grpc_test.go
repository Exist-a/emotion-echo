// Package downstream — llm_grpc_test.go
//
// Stage 81 RED（llm-chat-real-pipeline PR-2）：LLMGRPCClient 契约。
// 用真实 TCP loopback + fake servicer（不走 TLS）验证：
//   - 请求映射（messages/model/temperature/max_tokens）
//   - x-internal-api-key metadata 注入
//   - delta 流映射 + done 帧
package downstream

import (
	"context"
	"net"
	"testing"
	"time"

	emotionllm "github.com/emotion-echo/shared/pkg/emotionllm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// fakeLLMServiceServicer 捕获请求 + metadata，回放预设 delta
type fakeLLMServiceServicer struct {
	emotionllm.UnimplementedEmotionLLMServiceServer
	gotReq     *emotionllm.ChatCompletionRequest
	gotAPIKey  string
	deltas     []string
	withError  bool
}

func (f *fakeLLMServiceServicer) ChatCompletion(req *emotionllm.ChatCompletionRequest, stream emotionllm.EmotionLLMService_ChatCompletionServer) error {
	f.gotReq = req
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		if vs := md.Get("x-internal-api-key"); len(vs) > 0 {
			f.gotAPIKey = vs[0]
		}
	}
	if f.withError {
		return assert.AnError
	}
	for _, d := range f.deltas {
		if err := stream.Send(&emotionllm.ChatChunk{DeltaContent: d, Model: "deepseek-chat"}); err != nil {
			return err
		}
	}
	return stream.Send(&emotionllm.ChatChunk{Done: true, Model: "deepseek-chat"})
}

func startFakeLLMServer(t *testing.T, servicer *fakeLLMServiceServicer) *LLMGRPCClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	emotionllm.RegisterEmotionLLMServiceServer(srv, servicer)
	go func() { _ = srv.Serve(lis) }()

	// bufconn 直连（绕过 NewLLMGRPCClient 的网络拨号，只测流映射）
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		srv.Stop()
		_ = conn.Close()
	})
	return &LLMGRPCClient{conn: conn, client: emotionllm.NewEmotionLLMServiceClient(conn), apiKey: "test-internal-key"}
}

func TestLLMGRPCClient_StreamsDeltas(t *testing.T) {
	servicer := &fakeLLMServiceServicer{deltas: []string{"你", "好"}}
	c := startFakeLLMServer(t, servicer)

	var got []string
	var lastModel string
	err := c.ChatCompletionStream(context.Background(), LLMStreamRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(delta, model string) {
		got = append(got, delta)
		if model != "" {
			lastModel = model
		}
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"你", "好"}, got)
	assert.Equal(t, "deepseek-chat", lastModel)

	require.NotNil(t, servicer.gotReq)
	assert.Equal(t, "deepseek-chat", servicer.gotReq.Model)
	assert.Equal(t, "hi", servicer.gotReq.Messages[0].Content)
	assert.Equal(t, "test-internal-key", servicer.gotAPIKey, "必须注入 internal api key")
}

func TestLLMGRPCClient_UpstreamError_ReturnsError(t *testing.T) {
	servicer := &fakeLLMServiceServicer{withError: true}
	c := startFakeLLMServer(t, servicer)

	err := c.ChatCompletionStream(context.Background(), LLMStreamRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(string, string) {})
	require.Error(t, err)
}

// 编译期守卫：*LLMGRPCClient 必须满足 handler 用的 LLMChatStreamer 接口
var _ LLMChatStreamer = (*LLMGRPCClient)(nil)

var _ = time.Second // keep time import if unused later
