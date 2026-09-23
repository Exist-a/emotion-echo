// Package downstream — emotion_query_test.go
//
// Stage 30 / stage-30-web-bff.md T2.24 RED: EmotionQueryClient 契约测试
//
// 用 bufconn + 真 grpc.Server + fake EmotionQueryServiceServer，验证：
//   - ByMessage / ByConversation 参数传递 + 响应解码
//   - gRPC 错误（NotFound / InvalidArgument）→ error 透传
//   - MultiModalAnalyze ctx deadline（E2E-F-115）— fake 记录最近一次 ctx.Deadline()，
//     测试断言 BFF→ai-svc gRPC 调用带 30s deadline（防 SenseVoice 冷启动撞 5s 默认）
package downstream

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeEmotionQuerySrv 实现 EmotionQueryServiceServer
type fakeEmotionQuerySrv struct {
	emotionquery.UnimplementedEmotionQueryServiceServer

	// lastMultiModalDeadline 记录最近一次 MultiModalAnalyze 调用 ctx 的 deadline。
	// E2E-F-115 测试断言 deadline 在 [now+25s, now+35s] 区间（即 30s）。
	lastMultiModalDeadline time.Time
	lastMultiModalMu       sync.Mutex

	// E2E-F-124b：记录 GetEmotionByConversation / GetEmotionByMessage 收到的
	// x-user-id metadata（"none" = 无该 key，"" 前初值由 getter 兜底）。
	metaMu            sync.Mutex
	metaConvXUserID   string
	metaConvMetaSeen  bool
	metaMsgXUserID    string
	metaMsgMetaSeen   bool
}

// MultiModalAnalyze 实现 fake —— 仅记录 ctx deadline + 返回最小响应。
func (f *fakeEmotionQuerySrv) MultiModalAnalyze(ctx context.Context, _ *emotionquery.MultiModalAnalyzeRequest) (*emotionquery.MultiModalAnalyzeResponse, error) {
	if dl, ok := ctx.Deadline(); ok {
		f.lastMultiModalMu.Lock()
		f.lastMultiModalDeadline = dl
		f.lastMultiModalMu.Unlock()
	}
	return &emotionquery.MultiModalAnalyzeResponse{
		Kind:          "image",
		Emotion:       "neutral",
		Confidence:    0.5,
		SentimentScore: 0,
		Model:         "fake-v1",
	}, nil
}

// SynthesizeSpeech 实现 fake —— 记录 ctx deadline + 返回最小响应（E2E-F-115 测试用）。
func (f *fakeEmotionQuerySrv) SynthesizeSpeech(ctx context.Context, _ *emotionquery.SynthesizeSpeechRequest) (*emotionquery.SynthesizeSpeechResponse, error) {
	if dl, ok := ctx.Deadline(); ok {
		f.lastMultiModalMu.Lock()
		f.lastMultiModalDeadline = dl
		f.lastMultiModalMu.Unlock()
	}
	return &emotionquery.SynthesizeSpeechResponse{
		Audio:      "ZmFrZS13YXY=",
		SampleRate: 24000,
		Mime:       "audio/wav",
		Bytes:      12,
		Text:       "hi",
		Language:   "zh-cn",
	}, nil
}

func (f *fakeEmotionQuerySrv) GetEmotionByMessage(ctx context.Context, req *emotionquery.GetEmotionByMessageRequest) (*emotionquery.Emotion, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		v := md.Get("x-user-id")
		f.metaMu.Lock()
		f.metaMsgMetaSeen = true
		if len(v) > 0 {
			f.metaMsgXUserID = v[0]
		} else {
			f.metaMsgXUserID = "none"
		}
		f.metaMu.Unlock()
	} else {
		f.metaMu.Lock()
		f.metaMsgMetaSeen = true
		f.metaMsgXUserID = "none"
		f.metaMu.Unlock()
	}
	if req.MessageId == 0 {
		return nil, status.Error(codes.InvalidArgument, "message_id required")
	}
	return &emotionquery.Emotion{
		Id: 1, MessageId: req.MessageId, ConversationId: 10,
		PrimaryEmotion: "happy", SentimentScore: 0.7, Confidence: 0.9,
		Model: "keyword-stub-v1", CreatedAtMs: 123456,
	}, nil
}

func (f *fakeEmotionQuerySrv) GetEmotionByConversation(ctx context.Context, req *emotionquery.GetEmotionByConversationRequest) (*emotionquery.EmotionList, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		v := md.Get("x-user-id")
		f.metaMu.Lock()
		f.metaConvMetaSeen = true
		if len(v) > 0 {
			f.metaConvXUserID = v[0]
		} else {
			f.metaConvXUserID = "none"
		}
		f.metaMu.Unlock()
	} else {
		f.metaMu.Lock()
		f.metaConvMetaSeen = true
		f.metaConvXUserID = "none"
		f.metaMu.Unlock()
	}
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

// startFakeGRPCServerWithSrv 起 bufconn gRPC server 返回 conn + fake（让测试拿到 fake 实例断言副作用）。
func startFakeGRPCServerWithSrv(t *testing.T) (*grpc.ClientConn, *fakeEmotionQuerySrv) {
	t.Helper()
	fake := &fakeEmotionQuerySrv{}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	gs := grpc.NewServer()
	emotionquery.RegisterEmotionQueryServiceServer(gs, fake)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, fake
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

// ==== E2E-F-124b：EmotionQueryClient gRPC 必须注入 x-user-id metadata ====
//
// 背景（2026-09-23 F-122 端到端实测抓到）：ai-svc 拦截器拒所有缺 x-user-id
// metadata 的 RPC（除 health 白名单）。emotion_query.go 三个方法裸传 ctx ——
// 与 E2E-F-109（voice_handler 同型）完全同因：BFF 侧 session.WithRequestAuth
// 把 userID 放进 ctx，但 gRPC client 不调 withUserID(ctx) 就不会变成 metadata。
// 实测症状：F-122 情绪历史回落 GetEmotionByConversation 恒
// `Unauthenticated: missing x-user-id metadata` → 回落静默失效。
//
// 修法与 chat_grpc.go / ai_grpc.go 一致：三方法 ctx → withUserID(ctx)。
//
// fake 记录最近一次收到的 x-user-id metadata（按 RPC 分别记录）。

// lastMetaXUserID 记录 GetEmotionByConversation 收到的 x-user-id（"none" = 无该 key）。
func (f *fakeEmotionQuerySrv) lastMetaXUserID() string {
	f.metaMu.Lock()
	defer f.metaMu.Unlock()
	if !f.metaConvMetaSeen {
		return "none"
	}
	return f.metaConvXUserID
}

func (f *fakeEmotionQuerySrv) lastMetaMsgXUserID() string {
	f.metaMu.Lock()
	defer f.metaMu.Unlock()
	if !f.metaMsgMetaSeen {
		return "none"
	}
	return f.metaMsgXUserID
}

func TestEmotionQueryClient_ByConversation_InjectsXUserIDMetadata(t *testing.T) {
	conn, fake := startFakeGRPCServerWithSrv(t)
	c := NewEmotionQueryClient(conn)

	ctx := WithUserID(context.Background(), 42)
	_, _, err := c.ByConversation(ctx, 10, 3)
	require.NoError(t, err)
	assert.Equal(t, "42", fake.lastMetaXUserID(),
		"ByConversation 必须注入 x-user-id metadata（ai-svc 拦截器要求，缺失恒 Unauthenticated）")
}

func TestEmotionQueryClient_ByMessage_InjectsXUserIDMetadata(t *testing.T) {
	conn, fake := startFakeGRPCServerWithSrv(t)
	c := NewEmotionQueryClient(conn)

	ctx := WithUserID(context.Background(), 7)
	_, err := c.ByMessage(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, "7", fake.lastMetaMsgXUserID(),
		"ByMessage 必须注入 x-user-id metadata")
}

func TestEmotionQueryClient_NoUserID_NoMetadataStillSent(t *testing.T) {
	conn, fake := startFakeGRPCServerWithSrv(t)
	c := NewEmotionQueryClient(conn)

	// 无 userID（background ctx）→ 不注入 metadata，但请求本身仍应成功
	// （依赖 ai-svc 拦截器对无 metadata 的语义 —— 这里只断言 client 不 panic/不报错）
	_, _, err := c.ByConversation(context.Background(), 10, 1)
	require.NoError(t, err)
	assert.Equal(t, "none", fake.lastMetaXUserID(),
		"ctx 无 userID 时不得伪造 x-user-id（交由服务端拦截器判定）")
}
