// Package analyzer · auth wrapper
//
// AuthWrappedAnalyzer: 在调底层 analyzer 前自动注入 internal API key 到 ctx
// 复用 grpcinterceptor.WithInternalAPIKey。
//
// 设计动机：让 main.go 不需要重复写 metadata 注入逻辑。

package analyzer

import (
	"context"

	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

// AuthWrappedAnalyzer wraps an underlying Analyzer to inject an internal API key
// into the outgoing gRPC metadata (Stage 12 internal svc-to-svc auth).
//
// If apiKey is empty, no metadata is added (server auth disabled mode).
type AuthWrappedAnalyzer struct {
	inner  Analyzer
	apiKey string
}

// NewAuthWrappedAnalyzer creates a wrapper that injects apiKey into ctx.
func NewAuthWrappedAnalyzer(inner Analyzer, apiKey string) *AuthWrappedAnalyzer {
	return &AuthWrappedAnalyzer{inner: inner, apiKey: apiKey}
}

// Analyze calls inner.Analyze with ctx wrapped to include apiKey metadata.
func (a *AuthWrappedAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	if a.apiKey == "" {
		return a.inner.Analyze(ctx, text)
	}
	wrapped := grpcinterceptor.WithInternalAPIKey(ctx, a.apiKey)
	return a.inner.Analyze(wrapped, text)
}