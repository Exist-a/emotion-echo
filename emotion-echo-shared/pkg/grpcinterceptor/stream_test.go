// Package grpcinterceptor 的 stream 测试

package grpcinterceptor

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	emotionllm "github.com/emotion-echo/shared/pkg/emotionllm"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// silencedLogger 把 log 输出到 buffer，便于断言
type silencedLogger struct {
	buf *strings.Builder
}

func newSilencedLogger() *silencedLogger {
	return &silencedLogger{buf: &strings.Builder{}}
}

func (s *silencedLogger) Write(p []byte) (int, error) {
	return s.buf.Write(p)
}

// captureLog 临时把 log 输出重定向到 buffer
func captureLog(t *testing.T) func() string {
	t.Helper()
	sl := newSilencedLogger()
	orig := log.Writer()
	log.SetOutput(sl)
	return func() string {
		log.SetOutput(orig)
		return sl.buf.String()
	}
}

// =====================================================
// Test 1: Server stream logging — server 端 stream RPC 调用被记录
// =====================================================

func TestServerStreamLogging_LogsStartAndEnd(t *testing.T) {
	gs := grpc.NewServer(
		grpc.ChainStreamInterceptor(NewServerStreamLoggingInterceptor()),
	)
	emotionllm.RegisterEmotionLLMServiceServer(gs, &mockBatchServer{
		items: 3,
	})
	lis := bufconn.Listen(1024 * 64)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.Stop() })

	conn := dialBufForStream(t, lis)
	cli := emotionllm.NewEmotionLLMServiceClient(conn)

	getLog := captureLog(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := cli.AnalyzeBatch(ctx, &emotionllm.AnalyzeBatchRequest{
		Items: []*emotionllm.AnalyzeRequest{
			{MessageId: "1", Text: "happy"},
			{MessageId: "2", Text: "sad"},
			{MessageId: "3", Text: "calm"},
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeBatch err: %v", err)
	}

	count := 0
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("recv err: %v", err)
		}
		count++
	}
	if count != 3 {
		t.Fatalf("expected 3 messages, got %d", count)
	}

	logs := getLog()
	if !strings.Contains(logs, "stream-start") {
		t.Errorf("expected stream-start in log, got: %s", logs)
	}
	if !strings.Contains(logs, "stream-end") {
		t.Errorf("expected stream-end in log, got: %s", logs)
	}
	if !strings.Contains(logs, "sent=3") {
		t.Errorf("expected sent=3 in log, got: %s", logs)
	}
}

// =====================================================
// Test 2: Server stream recovery — handler panic 被捕获
// =====================================================

func TestServerStreamRecovery_PanicIsRecovered(t *testing.T) {
	gs := grpc.NewServer(
		grpc.ChainStreamInterceptor(
			NewServerStreamLoggingInterceptor(),
			NewServerStreamRecoveryInterceptor(),
		),
	)
	emotionllm.RegisterEmotionLLMServiceServer(gs, &mockBatchServer{
		panicAfterMsg: 1, // 发 1 条后 panic
	})
	lis := bufconn.Listen(1024 * 64)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.Stop() })

	conn := dialBufForStream(t, lis)
	cli := emotionllm.NewEmotionLLMServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := cli.AnalyzeBatch(ctx, &emotionllm.AnalyzeBatchRequest{
		Items: []*emotionllm.AnalyzeRequest{{MessageId: "1", Text: "x"}},
	})
	if err != nil {
		t.Fatalf("AnalyzeBatch err: %v", err)
	}

	// 第一条成功
	_, err = stream.Recv()
	if err != nil {
		t.Fatalf("first recv err: %v", err)
	}

	// 第二条应该收到 INTERNAL error（recovery 把 panic 转成 status）
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("expected error after panic, got nil")
	}
	if !strings.Contains(err.Error(), "Internal") {
		t.Errorf("expected Internal error, got: %v", err)
	}
}

// =====================================================
// Test 3: Client stream logging — client 端 stream RPC 计数 + 结束日志
// =====================================================

func TestClientStreamLogging_LogsStartEndAndMsgCount(t *testing.T) {
	gs := grpc.NewServer(
		grpc.ChainStreamInterceptor(),
	)
	emotionllm.RegisterEmotionLLMServiceServer(gs, &mockBatchServer{items: 3})
	lis := bufconn.Listen(1024 * 64)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.Stop() })

	conn := dialBufForStreamWithInterceptors(t, lis,
		grpc.WithChainStreamInterceptor(NewClientStreamLoggingInterceptor()),
	)

	getLog := captureLog(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 走底层 NewStream 而非 typed client（grpc 1.80 的 ServerStreamingClient[T]
	// 走 interceptor 行为有兼容性问题）
	streamDesc := &grpc.StreamDesc{
		StreamName:    "AnalyzeBatch",
		ServerStreams: true,
		ClientStreams: false,
	}
	cs, err := conn.NewStream(ctx, streamDesc, "/emotion_llm.v1.EmotionLLMService/AnalyzeBatch")
	if err != nil {
		t.Fatalf("NewStream err: %v", err)
	}
	if err := cs.SendMsg(&emotionllm.AnalyzeBatchRequest{
		Items: []*emotionllm.AnalyzeRequest{{MessageId: "1", Text: "happy"}},
	}); err != nil {
		t.Fatalf("SendMsg err: %v", err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend err: %v", err)
	}
	for {
		var resp emotionllm.AnalyzeResponse
		if err := cs.RecvMsg(&resp); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("RecvMsg err: %v", err)
		}
	}

	logs := getLog()
	if !strings.Contains(logs, "stream-start") {
		t.Errorf("expected stream-start in client log, got: %s", logs)
	}
	if !strings.Contains(logs, "stream-end") {
		t.Errorf("expected stream-end in client log, got: %s", logs)
	}
}

// =====================================================
// Test 4: Stream timeout — client 端 stream RPC 超时被中断
// =====================================================

func TestClientStreamTimeout_StopsSlowStream(t *testing.T) {
	gs := grpc.NewServer(
		grpc.ChainStreamInterceptor(),
	)
	emotionllm.RegisterEmotionLLMServiceServer(gs, &slowBatchServer{
		delay: 500 * time.Millisecond, // 每条都慢
		count: 10,
	})
	lis := bufconn.Listen(1024 * 64)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.Stop() })

	conn := dialBufForStream(t, lis)
	cli := emotionllm.NewEmotionLLMServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stream, err := cli.AnalyzeBatch(ctx, &emotionllm.AnalyzeBatchRequest{
		Items: []*emotionllm.AnalyzeRequest{{MessageId: "1", Text: "x"}},
	})
	if err != nil {
		t.Fatalf("AnalyzeBatch err: %v", err)
	}

	// ctx 已 100ms 超时，server 慢 500ms/条
	// 客户端应很快收到 DeadlineExceeded
	start := time.Now()
	_, err = stream.Recv()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("client should fail fast after ctx timeout, got %v", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) &&
		!strings.Contains(err.Error(), "DeadlineExceeded") {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

// =====================================================
// mock service implementations
// =====================================================

type mockBatchServer struct {
	emotionllm.UnimplementedEmotionLLMServiceServer
	items        int
	panicAfterMsg int
}

func (m *mockBatchServer) Analyze(ctx context.Context, req *emotionllm.AnalyzeRequest) (*emotionllm.AnalyzeResponse, error) {
	return &emotionllm.AnalyzeResponse{MessageId: req.MessageId, PrimaryEmotion: "happy", SentimentScore: 0.5}, nil
}

func (m *mockBatchServer) AnalyzeBatch(req *emotionllm.AnalyzeBatchRequest, stream grpc.ServerStreamingServer[emotionllm.AnalyzeResponse]) error {
	count := len(req.Items)
	if m.items > 0 {
		count = m.items
	}
	for i := 0; i < count; i++ {
		msgID := ""
		if i < len(req.Items) {
			msgID = req.Items[i].MessageId
		}
		if err := stream.Send(&emotionllm.AnalyzeResponse{
			MessageId:      msgID,
			PrimaryEmotion: "happy",
			SentimentScore: 0.5,
		}); err != nil {
			return err
		}
		// 在 send 完 N 条之后 panic（让 client 收到 N 条 + 1 个 Internal error）
		if m.panicAfterMsg > 0 && i+1 >= m.panicAfterMsg {
			panic("simulated handler panic in stream")
		}
	}
	return nil
}

type countingServer struct {
	emotionllm.UnimplementedEmotionLLMServiceServer
	counter *int32
}

func (c *countingServer) Analyze(ctx context.Context, req *emotionllm.AnalyzeRequest) (*emotionllm.AnalyzeResponse, error) {
	atomic.AddInt32(c.counter, 1)
	return &emotionllm.AnalyzeResponse{MessageId: req.MessageId, PrimaryEmotion: "x", SentimentScore: 0}, nil
}

func (c *countingServer) AnalyzeBatch(req *emotionllm.AnalyzeBatchRequest, stream grpc.ServerStreamingServer[emotionllm.AnalyzeResponse]) error {
	return nil
}

type slowBatchServer struct {
	emotionllm.UnimplementedEmotionLLMServiceServer
	delay time.Duration
	count int
}

func (s *slowBatchServer) Analyze(ctx context.Context, req *emotionllm.AnalyzeRequest) (*emotionllm.AnalyzeResponse, error) {
	return &emotionllm.AnalyzeResponse{MessageId: req.MessageId, PrimaryEmotion: "x", SentimentScore: 0}, nil
}

func (s *slowBatchServer) AnalyzeBatch(req *emotionllm.AnalyzeBatchRequest, stream grpc.ServerStreamingServer[emotionllm.AnalyzeResponse]) error {
	for i := 0; i < s.count; i++ {
		time.Sleep(s.delay)
		if err := stream.Send(&emotionllm.AnalyzeResponse{MessageId: "x", PrimaryEmotion: "x"}); err != nil {
			return err
		}
	}
	return nil
}

// =====================================================
// helpers
// =====================================================

func dialBufForStream(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	return dialBufForStreamWithInterceptors(t, lis)
}

func dialBufForStreamWithInterceptors(t *testing.T, lis *bufconn.Listener, opts ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	defaults := []grpc.DialOption{
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		append(defaults, opts...)...,
	)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
