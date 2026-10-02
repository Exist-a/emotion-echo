package grpcinterceptor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SkyAPM/go2sky"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

// TestGo2SkySpan_EndSpan_NilSpan_NoPanic Nil 底层 span 时不 panic
func TestGo2SkySpan_EndSpan_NilSpan_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EndSpan on nil span should not panic, got %v", r)
		}
	}()
	gs := &Go2SkySpan{span: nil}
	gs.EndSpan(nil)
	gs.EndSpan(errors.New("err"))
}

// TestGo2SkyTracer_NewGo2SkyTracer_NilReturnsNil nil tracer 应返回 nil
func TestGo2SkyTracer_NewGo2SkyTracer_NilReturnsNil(t *testing.T) {
	got := NewGo2SkyTracer(nil)
	if got != nil {
		t.Fatalf("NewGo2SkyTracer(nil) want nil, got %+v", got)
	}
}

// TestGo2SkyTracer_NewGo2SkyTracer_NonNilReturnsWrapped 非 nil 应包装
func TestGo2SkyTracer_NewGo2SkyTracer_NonNilReturnsWrapped(t *testing.T) {
	// 由于构造一个真 go2sky.Tracer 需要 reporter，最小化方法：直接 new 一个 zero-value
	// go2sky.NewTracer 在无 reporter 时不会 panic，返回带 nil reporter 的 tracer
	tr, err := go2sky.NewTracer("test-svc")
	if err != nil {
		// 有些版本 NewTracer 必传 reporter，这里跳过
		t.Skipf("go2sky.NewTracer without reporter unsupported in this version: %v", err)
	}
	got := NewGo2SkyTracer(tr)
	if got == nil {
		t.Fatalf("NewGo2SkyTracer(non-nil) should not return nil")
	}
	if got.tracer != tr {
		t.Fatalf("tracer pointer should round-trip")
	}
}

// TestGo2SkyTracer_StartEntry_NilReceiver_ReturnsNoopSpan nil receiver 返回 noop span + 原 ctx
func TestGo2SkyTracer_StartEntry_NilReceiver_ReturnsNoopSpan(t *testing.T) {
	var tr *Go2SkyTracer // nil
	ctx, sp := tr.StartEntry(context.Background(), "op")
	if ctx == nil {
		t.Fatalf("ctx should not be nil")
	}
	if sp == nil {
		t.Fatalf("span should not be nil")
	}
	if gs, ok := sp.(*Go2SkySpan); !ok {
		t.Fatalf("span should be *Go2SkySpan, got %T", sp)
	} else if gs.span != nil {
		t.Fatalf("noop span should have nil inner span, got non-nil")
	}
	// 调用 EndSpan 不应 panic
	sp.EndSpan(nil)
}

// TestGo2SkySpan_EndSpan_WithNonNilSpan_CallsError 当 inner span 非 nil 且 err != nil 时应记录错误
func TestGo2SkySpan_EndSpan_WithNonNilSpan_CallsError(t *testing.T) {
	// 构造一个 fakeSpan 实现 go2sky.Span 接口并断言 EndSpan 调用 sequence
	fs := &fakeGo2SkySpan{}
	gs := &Go2SkySpan{span: fs}
	gs.EndSpan(errors.New("test-error"))
	if fs.errorCount != 1 {
		t.Fatalf("expected 1 Error call, got %d", fs.errorCount)
	}
	if fs.errorMsg != "test-error" {
		t.Fatalf("error msg mismatch: %q", fs.errorMsg)
	}
	if fs.endCount != 1 {
		t.Fatalf("End should be called once even on error, got %d", fs.endCount)
	}
}

// TestGo2SkySpan_EndSpan_NoError_JumpsToEnd
func TestGo2SkySpan_EndSpan_NoError_JumpsToEnd(t *testing.T) {
	fs := &fakeGo2SkySpan{}
	gs := &Go2SkySpan{span: fs}
	gs.EndSpan(nil)
	if fs.errorCount != 0 {
		t.Fatalf("expected 0 Error calls, got %d", fs.errorCount)
	}
	if fs.endCount != 1 {
		t.Fatalf("expected 1 End call, got %d", fs.endCount)
	}
}

// TestGo2SkyTracer_StartEntry_OperatesOnExistingCtx 已注入 ctx 不被破坏
func TestGo2SkyTracer_StartEntry_OperatesOnExistingCtx(t *testing.T) {
	// 用 nil tracer 与 nil receiver：StartEntry 不动 ctx
	var tr *Go2SkyTracer
	type k struct{}
	parent := context.WithValue(context.Background(), k{}, "v")
	ctx, _ := tr.StartEntry(parent, "op")
	if ctx.Value(k{}) != "v" {
		t.Fatalf("ctx value should be preserved")
	}
}

// TestGo2SkyTracer_StartEntry_ReturnsSpanBearingCtx E2E-26 #3：
// StartEntry 必须返回**承载 span 的新 ctx** —— gin handler 拿它调下游 gRPC/Kafka，
// exit span 才能续上同一 trace。返回原 ctx ⇒ HTTP 入口 span 成孤岛，
// 每个下游 exit 各起新 trace（OAP 实测 web-bff 只有孤岛 gin trace 的根因之一）。
func TestGo2SkyTracer_StartEntry_ReturnsSpanBearingCtx(t *testing.T) {
	realTr, err := go2sky.NewTracer("startentry-ctx-test")
	if err != nil {
		t.Skipf("go2sky.NewTracer unsupported without reporter: %v", err)
	}
	tr := NewGo2SkyTracer(realTr)
	if tr == nil {
		t.Fatal("NewGo2SkyTracer(non-nil) 不应为 nil")
	}

	bg := context.Background()
	ctx1, sp := tr.StartEntry(bg, "GET /api/test")
	if sp == nil {
		t.Fatal("StartEntry 应返回非 nil span")
	}
	if ctx1 == bg {
		t.Fatalf("StartEntry 必须返回承载 span 的新 ctx（当前返回原 ctx ⇒ 下游 exit 每跳各起新 trace）")
	}
	// 承载 ctx 必须保留原 ctx 的值（X-Trace-Id 等已注入的字段）
	type k struct{}
	parent := context.WithValue(bg, k{}, "v")
	ctx2, _ := tr.StartEntry(parent, "op")
	if ctx2.Value(k{}) != "v" {
		t.Fatalf("承载 ctx 必须保留原 ctx 的值")
	}
}

// fakeGo2SkySpan 实现 go2sky.Span 接口
type fakeGo2SkySpan struct {
	endCount   int
	errorCount int
	errorMsg   string
}

func (s *fakeGo2SkySpan) End()                                       { s.endCount++ }
func (s *fakeGo2SkySpan) SetOperationName(string)                    {}
func (s *fakeGo2SkySpan) SetPeer(string)                              {}
func (s *fakeGo2SkySpan) GetOperationName() string                    { return "" }
func (s *fakeGo2SkySpan) SetSpanLayer(agentv3.SpanLayer)              {}
func (s *fakeGo2SkySpan) SetComponent(int32)                          {}
func (s *fakeGo2SkySpan) Tag(go2sky.Tag, string)                      {}
func (s *fakeGo2SkySpan) Log(_ time.Time, _ ...string)                {}
func (s *fakeGo2SkySpan) IsEntry() bool                               { return false }
func (s *fakeGo2SkySpan) IsExit() bool                                { return false }
func (s *fakeGo2SkySpan) IsValid() bool                               { return true }
func (s *fakeGo2SkySpan) Error(_ time.Time, args ...string) {
	s.errorCount++
	if len(args) > 0 {
		s.errorMsg = args[0]
	}
}
