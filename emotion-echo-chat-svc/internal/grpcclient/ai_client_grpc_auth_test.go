package grpcclient

// CL-F-66：chat-svc → ai-svc gRPC 必须带 x-internal-api-key。
//
// 背景：ai-svc 的 8892 改为强制内部鉴权后，所有调用方（BFF / chat-svc）不带 key 会全 401。
// 本测试用真实 gRPC server 捕获 incoming metadata，钉住"客户端真的发了"。

import (
	"context"
	"net"
	"testing"
	"time"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type captureAuthServer struct {
	emotionquery.UnimplementedEmotionQueryServiceServer
	keyCh chan string
}

func (s *captureAuthServer) UpsertNeutralEmotion(ctx context.Context, _ *emotionquery.UpsertNeutralEmotionRequest) (*emotionquery.UpsertNeutralEmotionResponse, error) {
	key := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-internal-api-key"); len(v) > 0 {
			key = v[0]
		}
	}
	s.keyCh <- key
	return &emotionquery.UpsertNeutralEmotionResponse{}, nil
}

func TestNewAIgRPCClient_SendsInternalAPIKey(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	impl := &captureAuthServer{keyCh: make(chan string, 1)}
	emotionquery.RegisterEmotionQueryServiceServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	cli, err := NewAIgRPCClient(lis.Addr().String(), "secret-key")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = cli.UpsertNeutralEmotion(ctx, &emotionquery.UpsertNeutralEmotionRequest{})
	require.NoError(t, err)

	select {
	case got := <-impl.keyCh:
		require.Equal(t, "secret-key", got, "客户端必须携带 x-internal-api-key")
	case <-time.After(2 * time.Second):
		t.Fatal("server did not receive the call")
	}
}
