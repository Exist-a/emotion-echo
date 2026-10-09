// Package analyzer — auth_wrapped_test.go
//
// Sibling test for auth_wrapped.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover auth_wrapped.go (36 LOC) test
// surface. AuthWrappedAnalyzer delegates to an inner Analyzer; its
// sole job is to inject the internal API key into ctx via
// grpcinterceptor.WithInternalAPIKey before calling inner.Analyze.
//
// Coverage:
//
//   - empty apiKey → no metadata wrapping; ctx passed through verbatim
//   - non-empty apiKey → inner.Analyze sees a ctx with apiKey metadata
//   - inner.Analyze returns error → wrapper propagates verbatim
//   - inner.Analyze returns nil error with a result → wrapper forwards
//
// We use a small in-package stubAnalyzer (no snapshot-copy of the
// keyword dictionary) to capture the ctx the inner receives and
// verify the metadata was injected.
package analyzer

import (
	"context"
	"errors"
	"testing"

	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/metadata"
)

// ctxCapturingAnalyzer is a stub that records the ctx it was called
// with and returns a configurable result / error.
type ctxCapturingAnalyzer struct {
	gotCtx context.Context
	result *EmotionResult
	err    error
	called bool
}

func (s *ctxCapturingAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	s.called = true
	s.gotCtx = ctx
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &EmotionResult{PrimaryEmotion: "happy", Confidence: 1.0, Model: "stub"}, nil
}

// metadataHasInternalAPIKey 检查 ctx 是否携带 WithInternalAPIKey 注入的
// outgoing metadata。2026-10-09（E2E-31 第二方核对）补：此处原先的注释
// 声称"为省依赖不做 metadata 断言"，于是本文件**没有任何一条**测试真正验证
// "apiKey 被写进 ctx" —— 而随 E2E-31 T-2 删除 `GRPCAnalyzer.AnalyzeWithAuth`
// 时，那条**唯一**带 `md.Get("x-internal-api-key") == apiKey` 真断言的用例
// 也一并没了，形成真实覆盖缺口。现用 grpc-go 的 metadata 包直接断言。
func metadataHasInternalAPIKey(ctx context.Context, want string) (bool, []string) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return false, nil
	}
	vals := md.Get(grpcinterceptor.InternalAPIKeyMetadataKey)
	if len(vals) == 0 {
		return false, nil
	}
	return vals[0] == want, vals
}

func TestAuthWrappedAnalyzer_EmptyAPIKey_NoWrapping(t *testing.T) {
	t.Parallel()

	inner := &ctxCapturingAnalyzer{
		result: &EmotionResult{PrimaryEmotion: "happy", Confidence: 0.8, Model: "stub"},
	}
	w := NewAuthWrappedAnalyzer(inner, "")

	inputCtx := context.Background()
	out, err := w.Analyze(inputCtx, "hi")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.True(t, inner.called)
	assert.Equal(t, "happy", out.PrimaryEmotion)
	// 反向断言（与上一个用例配对）：空 apiKey ⇒ 不得注入 metadata
	// （WithInternalAPIKey 的"服务端鉴权关闭"语义）
	ok, vals := metadataHasInternalAPIKey(inner.gotCtx, "")
	assert.False(t, ok, "空 apiKey 不应注入 x-internal-api-key，实际=%v", vals)
}

func TestAuthWrappedAnalyzer_NonEmptyAPIKey_InnerCalledWithWrappedCtx(t *testing.T) {
	t.Parallel()

	inner := &ctxCapturingAnalyzer{
		result: &EmotionResult{PrimaryEmotion: "happy", Confidence: 0.8, Model: "stub"},
	}
	const apiKey = "internal-key-xyz"
	w := NewAuthWrappedAnalyzer(inner, apiKey)

	inputCtx := context.Background()
	out, err := w.Analyze(inputCtx, "hi")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.True(t, inner.called)
	assert.Equal(t, "happy", out.PrimaryEmotion)
	// 真断言（2026-10-09 补，E2E-31 第二方核对条件 (a)）：inner 收到的 ctx 必须
	// 携带 apiKey 的 outgoing metadata。这是 AuthWrappedAnalyzer 存在的**唯一理由**，
	// 旧版本只断言"没 panic"（自承无法比较 ctx）⇒ 属弱断言。
	ok, vals := metadataHasInternalAPIKey(inner.gotCtx, apiKey)
	assert.True(t, ok, "inner 收到的 ctx 应携带 x-internal-api-key 元数据，实际=%v", vals)

	// 反向：空 apiKey 路径不得注入（下面单独用例覆盖）
	assert.NotNil(t, inner.gotCtx)
}

func TestAuthWrappedAnalyzer_InnerError_PropagatesAsIs(t *testing.T) {
	t.Parallel()

	inner := &ctxCapturingAnalyzer{
		err: errors.New("downstream timeout"),
	}
	w := NewAuthWrappedAnalyzer(inner, "any-key")

	out, err := w.Analyze(context.Background(), "hi")
	assert.Nil(t, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downstream timeout")
}

func TestAuthWrappedAnalyzer_InnerSuccess_ResultForwarded(t *testing.T) {
	t.Parallel()

	inner := &ctxCapturingAnalyzer{
		result: &EmotionResult{
			PrimaryEmotion: "calm",
			SentimentScore: 0.4,
			Confidence:     0.95,
			Model:          "auth-stub",
		},
	}
	w := NewAuthWrappedAnalyzer(inner, "k")

	out, err := w.Analyze(context.Background(), "hi")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "calm", out.PrimaryEmotion)
	assert.Equal(t, "auth-stub", out.Model)
	assert.InDelta(t, 0.95, out.Confidence, 0.001)
}

// TestAuthWrappedAnalyzer_EmptyText_InnerStillCalled documents that
// the wrapper does NOT do its own validation — empty text passes
// through to the inner. (Validation is the responsibility of the
// caller, not the auth wrapper.) This pins the delegation contract.
func TestAuthWrappedAnalyzer_EmptyText_InnerStillCalled(t *testing.T) {
	t.Parallel()

	inner := &ctxCapturingAnalyzer{
		result: &EmotionResult{PrimaryEmotion: "neutral", Confidence: 0.5, Model: "stub"},
	}
	w := NewAuthWrappedAnalyzer(inner, "k")

	out, err := w.Analyze(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.True(t, inner.called, "wrapper must delegate even for empty text")
}
