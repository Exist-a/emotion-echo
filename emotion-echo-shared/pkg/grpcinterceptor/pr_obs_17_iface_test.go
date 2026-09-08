package grpcinterceptor

import (
	"context"
	"testing"
)

// ===== PR-OBS-17 RED: 接口合规断言 =====
//
// 强制 mockSpan / mockTracer 满足 Span / Tracer 接口（编译期检查）。
// Span/Tracer 接口扩 Tag/CreateLocalSpan 后，这些断言保证 mock 同步更新。

// 编译期断言：mockSpan 必须满足 Span 接口
var _ Span = (*mockSpan)(nil)

// 编译期断言：mockTracer 必须满足 Tracer 接口
var _ Tracer = (*mockTracer)(nil)

// TestMockSpanImplementsSpan_CompileTimeGuard 运行时 guard(编译失败已 RED)
func TestMockSpanImplementsSpan_CompileTimeGuard(t *testing.T) {
	t.Parallel()
	var s Span = &mockSpan{}
	s.Tag("k", "v")
	if got, ok := s.(*mockSpan); !ok || len(got.tagCalls) != 1 {
		t.Fatalf("Tag should be observable via interface, got %+v", got)
	}
}

// TestMockTracerImplementsTracer_CompileTimeGuard
func TestMockTracerImplementsTracer_CompileTimeGuard(t *testing.T) {
	t.Parallel()
	var tr Tracer = &mockTracer{localSpan: &mockSpan{}}
	ctx, span, err := tr.CreateLocalSpan(context.Background(), "op")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ctx == nil || span == nil {
		t.Fatal("ctx and span should be non-nil")
	}
}
