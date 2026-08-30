// Package downstream — emotion_query_test.go
//
// Stage 30 / stage-30-web-bff.md T2.24 RED: EmotionQueryClient 契约测试
//
// 用 bufconn + 真 grpc.Server + fake EmotionQueryServiceServer，验证：
//   - ByMessage / ByConversation 参数传递 + 响应解码
//   - gRPC 错误（NotFound / InvalidArgument）→ error 透传
package downstream

import (
	"context"
	"net"
	"testing"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// fakeEmotionQuerySrv 实现 EmotionQueryServiceServer
type fakeEmotionQuerySrv struct {
	emotionquery.UnimplementedEmotionQueryServiceServer
}

func (f *fakeEmotionQuerySrv) GetEmotionByMessage(_ context.Context, req *emotionquery.GetEmotionByMessageRequest) (*emotionquery.Emotion, error) {
	if req.MessageId == 0 {
		return nil, status.Error(codes.InvalidArgument, "message_id required")
	}
	return &emotionquery.Emotion{
		Id: 1, MessageId: req.MessageId, ConversationId: 10,
		PrimaryEmotion: "happy", SentimentScore: 0.7, Confidence: 0.9,
		Model: "keyword-stub-v1", CreatedAtMs: 123456,
	}, nil
}

func (f *fakeEmotionQuerySrv) GetEmotionByConversation(_ context.Context, req *emotionquery.GetEmotionByConversationRequest) (*emotionquery.EmotionList, error) {
	if req.ConversationId == 0 {
		return nil, status.Error(codes.InvalidArgument, "conversation_id required")
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 1
	}
	items := make([]*emotionquery.Emotion, 0, limit)
	for i := 0; i < limit; i++ {
		items = append(items, &emotionquery.Emotion{
			Id: int64(i + 1), MessageId: int64(100 + i), ConversationId: req.ConversationId,
			PrimaryEmotion: "calm",
		})
	}
	return &emotionquery.EmotionList{Items: items, Total: int32(len(items))}, nil
}

// startFakeGRPCServer 起 bufconn gRPC server，返回 conn + cleanup
func startFakeGRPCServer(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	gs := grpc.NewServer()
	emotionquery.RegisterEmotionQueryServiceServer(gs, &fakeEmotionQuerySrv{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestEmotionQueryClient_ByMessage_Success(t *testing.T) {
	conn := startFakeGRPCServer(t)
	c := NewEmotionQueryClient(conn)

	e, err := c.ByMessage(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, int64(42), e.MessageId)
	assert.Equal(t, "happy", e.PrimaryEmotion)
	assert.Equal(t, 0.7, e.SentimentScore)
	assert.Equal(t, int64(123456), e.CreatedAtMs)
}

func TestEmotionQueryClient_ByConversation_Success(t *testing.T) {
	conn := startFakeGRPCServer(t)
	c := NewEmotionQueryClient(conn)

	items, total, err := c.ByConversation(context.Background(), 10, 3)
	require.NoError(t, err)
	assert.Equal(t, int32(3), total)
	require.Len(t, items, 3)
	assert.Equal(t, "calm", items[0].PrimaryEmotion)
}

func TestEmotionQueryClient_ByMessage_InvalidArgument_ReturnsError(t *testing.T) {
	conn := startFakeGRPCServer(t)
	c := NewEmotionQueryClient(conn)

	_, err := c.ByMessage(context.Background(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message_id required", "gRPC InvalidArgument 应透传")
}

func TestEmotionQueryClient_ByConversation_InvalidArgument_ReturnsError(t *testing.T) {
	conn := startFakeGRPCServer(t)
	c := NewEmotionQueryClient(conn)

	_, _, err := c.ByConversation(context.Background(), 0, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conversation_id required")
}
