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
	"testing"

	emotionllm "github.com/emotion-echo/shared/pkg/emotionllm"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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