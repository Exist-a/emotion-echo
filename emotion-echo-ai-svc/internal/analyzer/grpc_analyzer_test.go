// Package analyzer — grpc_analyzer_test.go
//
// Sibling test for grpc_analyzer.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 coverage: grpc_analyzer.go (LOC=184) had
// no sibling test. The Analyze path is the gRPC bridge between
// ai-svc and the Python emotion-llm-service — a critical cross-
// language contract.
//
// We test the response mapping (AnalyzeResponse → EmotionResult)
// by injecting a fake EmotionLLMServiceClient. NewGRPCAnalyzer
// (dial + TLS + health check) is intentionally NOT tested here
// because it requires a real gRPC server; that belongs in
// //go:build integration tests.
//
// Coverage matrix:
//
//   - HappyPath: full EmotionResult mapping
//   - ClientError: gRPC error propagates wrapped with "grpc analyze failed"
//   - EmptyResponse: zero-valued AnalyzeResponse yields zero-value EmotionResult
//   - RequestTextForwarded: input text reaches AnalyzeRequest
//   - AnalyzeWithAuth: ctx is enriched via WithInternalAPIKey (the
//     resulting ctx carries the metadata)
//   - Close: nil-safe (conn==nil returns nil)
package analyzer

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	emotionllm "github.com/emotion-echo/shared/pkg/emotionllm"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// fakeEmotionLLMClient implements emotionllm.EmotionLLMServiceClient.
type fakeEmotionLLMClient struct {
	resp *emotionllm.AnalyzeResponse
	err  error
	// lastReq captures the request the analyzer sent so tests can
	// assert on ctx propagation (API key etc.).
	lastReq *emotionllm.AnalyzeRequest
	// lastCtx captures the ctx (used by AnalyzeWithAuth test).
	lastCtx context.Context
}

func (f *fakeEmotionLLMClient) Analyze(ctx context.Context, in *emotionllm.AnalyzeRequest, _ ...grpc.CallOption) (*emotionllm.AnalyzeResponse, error) {
	f.lastReq = in
	f.lastCtx = ctx
	return f.resp, f.err
}

// AnalyzeBatch returns nil + error to satisfy the interface; we don't
// exercise the batch path in this test surface.
func (f *fakeEmotionLLMClient) AnalyzeBatch(_ context.Context, _ *emotionllm.AnalyzeBatchRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[emotionllm.AnalyzeResponse], error) {
	return nil, errors.New("AnalyzeBatch not implemented in fake")
}

// ChatCompletion Stage 80 后 EmotionLLMService 新增的 RPC（llm-chat-real-pipeline
// PR-1）；本测试 surface 不覆盖 ChatCompletion 路径，仅补接口合规避免 build fail。
func (f *fakeEmotionLLMClient) ChatCompletion(_ context.Context, _ *emotionllm.ChatCompletionRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[emotionllm.ChatChunk], error) {
	return nil, errors.New("ChatCompletion not implemented in fake")
}

// ClassifyIntent Stage 82/87 后 EmotionLLMService 新增的 RPC（intent-classification
// 6-types）；本测试 surface 不覆盖此路径，仅补接口合规避免 build fail。
func (f *fakeEmotionLLMClient) ClassifyIntent(_ context.Context, _ *emotionllm.ClassifyIntentRequest, _ ...grpc.CallOption) (*emotionllm.IntentResult, error) {
	return nil, errors.New("ClassifyIntent not implemented in fake")
}

func TestGRPCAnalyzer_Analyze_HappyPath_MapsResponse(t *testing.T) {
	t.Parallel()
	fake := &fakeEmotionLLMClient{
		resp: &emotionllm.AnalyzeResponse{
			PrimaryEmotion: "happy",
			SentimentScore: 0.85,
			Confidence:     0.92,
			Model:          "keyword-v1",
		},
	}
	a := &GRPCAnalyzer{client: fake}

	got, err := a.Analyze(context.Background(), "I love this!")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.InDelta(t, 0.85, got.SentimentScore, 0.001)
	assert.InDelta(t, 0.92, got.Confidence, 0.001)
	assert.Equal(t, "keyword-v1", got.Model)
}

func TestGRPCAnalyzer_Analyze_ClientError_WrapsWithMessage(t *testing.T) {
	t.Parallel()
	boom := errors.New("connection refused")
	fake := &fakeEmotionLLMClient{err: boom}
	a := &GRPCAnalyzer{client: fake}

	_, err := a.Analyze(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "grpc analyze failed")
	assert.ErrorIs(t, err, boom)
}

func TestGRPCAnalyzer_Analyze_EmptyResponse_ZeroValues(t *testing.T) {
	t.Parallel()
	fake := &fakeEmotionLLMClient{resp: &emotionllm.AnalyzeResponse{}}
	a := &GRPCAnalyzer{client: fake}

	got, err := a.Analyze(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "", got.PrimaryEmotion)
	assert.Equal(t, 0.0, got.SentimentScore)
	assert.Equal(t, 0.0, got.Confidence)
}

func TestGRPCAnalyzer_Analyze_RequestTextForwarded(t *testing.T) {
	t.Parallel()
	fake := &fakeEmotionLLMClient{
		resp: &emotionllm.AnalyzeResponse{PrimaryEmotion: "ok"},
	}
	a := &GRPCAnalyzer{client: fake}

	_, err := a.Analyze(context.Background(), "specific text input")
	require.NoError(t, err)
	require.NotNil(t, fake.lastReq)
	assert.Equal(t, "specific text input", fake.lastReq.GetText())
}

func TestGRPCAnalyzer_AnalyzeWithAuth_InjectsAPIKeyMetadata(t *testing.T) {
	t.Parallel()
	fake := &fakeEmotionLLMClient{
		resp: &emotionllm.AnalyzeResponse{PrimaryEmotion: "ok"},
	}
	a := &GRPCAnalyzer{client: fake}

	const apiKey = "test-internal-api-key-xyz"
	_, err := a.AnalyzeWithAuth(context.Background(), "text", apiKey)
	require.NoError(t, err)

	// The resulting ctx (as seen by Analyze) must contain the
	// "x-internal-api-key" metadata injected by WithInternalAPIKey.
	md, ok := metadata.FromOutgoingContext(fake.lastCtx)
	if !ok {
		t.Fatalf("AnalyzeWithAuth did not produce an outgoing metadata ctx; got %v", fake.lastCtx)
	}
	keys := md.Get("x-internal-api-key")
	require.NotEmpty(t, keys, "expected x-internal-api-key metadata to be set")
	assert.Equal(t, apiKey, keys[0])
}

func TestGRPCAnalyzer_AnalyzeWithAuth_EmptyAPIKey_StillProducesContext(t *testing.T) {
	t.Parallel()
	fake := &fakeEmotionLLMClient{
		resp: &emotionllm.AnalyzeResponse{PrimaryEmotion: "ok"},
	}
	a := &GRPCAnalyzer{client: fake}

	_, err := a.AnalyzeWithAuth(context.Background(), "text", "")
	require.NoError(t, err)
	// Empty key path: WithInternalAPIKey skips metadata injection
	// (per its own behavior). The fake still receives a non-nil
	// ctx and the call succeeds.
	assert.NotNil(t, fake.lastCtx)
}

func TestGRPCAnalyzer_Close_NilConn_ReturnsNil(t *testing.T) {
	t.Parallel()
	a := &GRPCAnalyzer{conn: nil}
	assert.NoError(t, a.Close())
}

// reference: keeps the import live if grpcinterceptor.WithInternalAPIKey
// is the only thing pulled from this package.
var _ = grpcinterceptor.WithInternalAPIKey

// =====================================================
// Stage 94 PR-2 §P0-2 RED · ai-svc → llm-service gRPC client sw8 metadata 透传
// =====================================================
//
// Stage 94 PR-3 已修 NewClientTracingInterceptor 用 CreateExitSpan + metadata.MD
// 注入 sw8。本测试验证 ai-svc grpc_analyzer.go 通过 ClientDialOptions helper 接入
// 后,chat-svc → ai-svc → llm-service 的 trace 链能继续跨 gRPC 进程透传
// (server 端 EmotionLLMService.Analyze handler 从 incoming metadata 抽到 sw8)。
//
// 测试设计：bufconn 起 grpc server + 注册 EmotionLLMService.Analyze 实现
// (UnimplementedEmotionLLMServiceServer embed 满足接口合规) → 走 ClientDialOptions
// (tracer + 3s timeout) dial → 调 Analyze → server 端读 incoming metadata 验证
// "sw8" header 存在(由 Stage 94 PR-3 修复后的 NewClientTracingInterceptor 注入)。
//
// 旧实现：grpc_analyzer.go 直接 inline `grpc.WithChainUnaryInterceptor(...)`
// (4 个 interceptor),虽然功能等价,但有重复样板,timeout 是魔法数字 3s。
// 本测试钉死契约：重构为 ClientDialOptions 后行为不变 + sw8 仍透传。
func TestGRPCAnalyzer_ClientDialOptions_PropagatesSw8MetadataToServer(t *testing.T) {
	t.Parallel()

	// 1. bufconn listener
	lis := bufconn.Listen(1024 * 64)

	// 2. grpc server + register EmotionLLMService.Analyze handler that
	//    captures incoming metadata
	var gotSw8 string
	var mdMu sync.Mutex
	srv := grpc.NewServer()
	emotionllm.RegisterEmotionLLMServiceServer(srv, &captureSw8Server{
		onAnalyze: func(ctx context.Context) {
			mdMu.Lock()
			defer mdMu.Unlock()
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				gotSw8 = md.Get("sw8")[0] // 注入由 Stage 94 PR-3 修复后的 interceptor 完成
			}
		},
	})

	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	// 3. ai-svc GRPCAnalyzer dial via bufconn with ClientDialOptions
	//    关键：不再用 NewGRPCAnalyzer (走 TCP + health check),而直接构造 conn
	//    这样可以注入 bufconn dialer + ClientDialOptions
	const fakeSw8 = "1-p02-ai-svc-grpc-analyzer-e2e"
	tracer := &captureSw8Tracer{sw8ToInject: fakeSw8}
	dialOpts := grpcinterceptor.ClientDialOptions(tracer, 3*time.Second)
	conn, err := grpc.NewClient("passthrough://bufnet",
		append([]grpc.DialOption{
			grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
				return lis.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}, dialOpts...)...,
	)
	require.NoError(t, err)
	defer conn.Close()

	// 4. 走 Analyze RPC,server 端应能抽到 sw8
	cli := emotionllm.NewEmotionLLMServiceClient(conn)
	_, _ = cli.Analyze(context.Background(), &emotionllm.AnalyzeRequest{Text: "hi"})

	mdMu.Lock()
	defer mdMu.Unlock()
	if gotSw8 != fakeSw8 {
		t.Errorf("server 端抽到的 sw8 = %q, want %q（§P0-2 修复前 NewClientTracingInterceptor 不透传 sw8 metadata）",
			gotSw8, fakeSw8)
	}
}

// captureSw8Server 满足 emotionllm.EmotionLLMServiceServer 接口,只覆盖
// Analyze 用于 e2e metadata 透传断言;其他方法走 Unimplemented。
type captureSw8Server struct {
	emotionllm.UnimplementedEmotionLLMServiceServer
	onAnalyze func(ctx context.Context)
}

func (s *captureSw8Server) Analyze(ctx context.Context, _ *emotionllm.AnalyzeRequest) (*emotionllm.AnalyzeResponse, error) {
	if s.onAnalyze != nil {
		s.onAnalyze(ctx)
	}
	return &emotionllm.AnalyzeResponse{PrimaryEmotion: "ok"}, nil
}

// captureSw8Tracer 是 grpcinterceptor.Tracer 的最小 mock：CreateExitSpan
// 时把 fakeSw8 通过 injector 写入 outgoing metadata,实现 sw8 header 注入。
// (Stage 94 PR-3 修复后 NewClientTracingInterceptor 内部走 metadata.NewOutgoingContext)
//
// 必须完整实现 4 个方法（StartEntry / CreateLocalSpan / CreateExitSpan / CreateEntrySpan），
// 否则编译期 grpcinterceptor.Tracer 接口断言失败。
type captureSw8Tracer struct {
	sw8ToInject string
}

func (t *captureSw8Tracer) StartEntry(ctx context.Context, _ string) (context.Context, grpcinterceptor.Span) {
	return ctx, &captureSw8Span{}
}
func (t *captureSw8Tracer) CreateLocalSpan(ctx context.Context, _ string) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &captureSw8Span{}, nil
}
func (t *captureSw8Tracer) CreateExitSpan(
	_ context.Context, _ string, _ string,
	injector func(string, string) error,
) (context.Context, grpcinterceptor.Span, error) {
	if injector != nil {
		_ = injector("sw8", t.sw8ToInject)
	}
	return context.Background(), &captureSw8Span{}, nil
}
func (t *captureSw8Tracer) CreateEntrySpan(ctx context.Context, _ string, _ func(string) (string, error)) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &captureSw8Span{}, nil
}

// 编译期断言 captureSw8Tracer 满足 grpcinterceptor.Tracer 接口
var _ grpcinterceptor.Tracer = (*captureSw8Tracer)(nil)

// captureSw8Span 满足 grpcinterceptor.Span 接口,不做事(仅让 EndSpan 调用不 panic)
type captureSw8Span struct{}

func (*captureSw8Span) EndSpan(_ error)                                {}
func (*captureSw8Span) Tag(_ string, _ string)                         {}
func (*captureSw8Span) SetComponent(_ int32)                           {}
func (*captureSw8Span) SetSpanLayer(_ int32)                           {}

// TestGRPCAnalyzer_UsesClientDialOptionsHelper §P0-2 钉死契约:
//
// grpc_analyzer.go 的 dial options 必须经 sharedgrpc.ClientDialOptions helper
// 装配(链顺序: tracing → timeout → logging → retry),而不是 inline 4 个
// interceptor 直接 chain。理由:
//   - 5 svc (chat/ai/analytics/bff/llm) dial 模式复用同一 helper,改一处生效
//   - §P0-2 修复让 chat-svc → ai-svc → llm-service 的 sw8 链贯通
//     (与 BFF §P0-1b PR-4 对齐)
//   - 与 ai-svc → llm-service grpc_analyzer.go 现状 4 个 interceptor 等价,
//     但用 helper 表达"tracing + timeout + logging + retry"业务意图
//
// 本测试用源码字面量断言（与 Stage 92 §"tag 字面量未漂移"同模式）——
// 与 e2e metadata 断言互补：e2e 验"行为正确"，字面量断言钉死"用 helper 而非复制"。
func TestGRPCAnalyzer_UsesClientDialOptionsHelper(t *testing.T) {
	srcBytes, err := os.ReadFile("grpc_analyzer.go")
	if err != nil {
		t.Skipf("cannot read grpc_analyzer.go: %v", err)
	}
	src := string(srcBytes)

	// §P0-2 修复目标:grpc_analyzer.go 必须调 shared ClientDialOptions 装配
	// chain,而不是 inline 重复 4 个 interceptor。
	wantContain := []string{
		// 必须用 helper（行为一致的复用入口）
		`grpcinterceptor.ClientDialOptions(`,
		// helper 已包含 tracing+timeout+logging;retry 是 ai-svc 业务特化,
		// 在 helper 之外单独 append(与 ai-svc 之前 4 interceptor 链顺序一致)
		`grpcinterceptor.ClientRetryInterceptor(`,
		// 必须给 helper 传 tracer —— 用 go2sky adapter 包装（生产模式）
		`grpcinterceptor.NewGo2SkyTracer(`,
	}
	for _, w := range wantContain {
		if !strings.Contains(src, w) {
			t.Errorf("grpc_analyzer.go 缺 %q — §P0-2 修复要求用 shared ClientDialOptions helper 而非 inline 4 个 interceptor",
				w)
		}
	}

	// 反向:旧 inline 模式不再出现(避免本次重构只部分完成 —— retry 与 logging
	// 仍直接调,tracing 没走 helper)
	notWant := []string{
		`grpcinterceptor.NewClientTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(traceTracer()))`,
	}
	for _, nw := range notWant {
		if strings.Contains(src, nw) {
			t.Errorf("grpc_analyzer.go 仍含旧 inline 模式 %q —— 应改用 ClientDialOptions helper（§P0-2 修复未完成）", nw)
		}
	}
}