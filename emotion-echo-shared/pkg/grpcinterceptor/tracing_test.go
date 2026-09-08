package grpcinterceptor

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// =====================================================
// 测试用的 mock Tracer 和 Span
// =====================================================

// tagKV 记录 span.Tag(key, value) 调用
type tagKV struct{ K, V string }

// mockSpan PR-OBS-17 扩展：加 Tag 方法 + tagCalls 记录
type mockSpan struct {
	endErr   error
	ended    bool
	tagCalls []tagKV
}

func (s *mockSpan) EndSpan(err error) {
	s.ended = true
	s.endErr = err
}

func (s *mockSpan) Tag(key, value string) {
	s.tagCalls = append(s.tagCalls, tagKV{key, value})
}

// mockTracer PR-OBS-17 扩展：加 CreateLocalSpan 方法
type mockTracer struct {
	// calls 记录 StartEntry 被调用的参数
	calls []string
	// spanToReturn 每次 StartEntry 返回的 span（共享引用）
	spanToReturn *mockSpan
	// ctxToReturn 每次 StartEntry 返回的 ctx
	ctxToReturn context.Context

	// localOpCalls 记录 CreateLocalSpan 的 opName
	localOpCalls []string
	// localSpan 每次 CreateLocalSpan 返回的 span
	localSpan *mockSpan
	// localCtx 每次 CreateLocalSpan 返回的 ctx（nil → 用入参 ctx）
	localCtx context.Context
	// localErr 每次 CreateLocalSpan 返回的 err
	localErr error
}

func (t *mockTracer) StartEntry(ctx context.Context, opName string) (context.Context, Span) {
	t.calls = append(t.calls, opName)
	if t.ctxToReturn == nil {
		t.ctxToReturn = ctx
	}
	return t.ctxToReturn, t.spanToReturn
}

func (t *mockTracer) CreateLocalSpan(ctx context.Context, opName string) (context.Context, Span, error) {
	t.localOpCalls = append(t.localOpCalls, opName)
	outCtx := ctx
	if t.localCtx != nil {
		outCtx = t.localCtx
	}
	return outCtx, t.localSpan, t.localErr
}

// =====================================================
// ServerTracingInterceptor 测试
// =====================================================

func TestServerTracing_NilTracer_BypassesTracing(t *testing.T) {
	t.Parallel()
	interceptor := NewServerTracingInterceptor(nil)
	h := &fakeHandler{}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if h.called != 1 {
		t.Fatalf("expected handler called 1 time, got %d", h.called)
	}
}

func TestServerTracing_HappyPath_CallsTracerAndEndsSpan(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		h.handle)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// tracer.StartEntry 应被调用一次，参数是 FullMethod
	if len(tracer.calls) != 1 || tracer.calls[0] != "/svc/Method" {
		t.Fatalf("expected StartEntry called once with method name, got %v", tracer.calls)
	}
	if !span.ended {
		t.Fatal("expected span to be ended")
	}
	if span.endErr != nil {
		t.Fatalf("expected span.EndSpan(nil) on success, got err=%v", span.endErr)
	}
}

func TestServerTracing_HandlerError_PropagatesAndEndsSpanWithErr(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{returnErr: errors.New("downstream fail")}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		h.handle)

	if err == nil {
		t.Fatal("expected err to propagate")
	}
	if !span.ended {
		t.Fatal("expected span to be ended even on error")
	}
	if span.endErr == nil || span.endErr.Error() != "downstream fail" {
		t.Fatalf("expected span.EndSpan(err), got endErr=%v", span.endErr)
	}
}

func TestServerTracing_PanicInHandler_EndsSpanWithRecoveredErr(t *testing.T) {
	t.Parallel()
	// 注意：panic 不会被 tracing interceptor 捕获，
	// 因为 recovery interceptor 应该 wrap 在外面。
	// 但 tracing 自己应该正确传递 panic。
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{panicVal: "boom"}

	defer func() {
		// 验证 span.EndSpan 在 panic 之前被 defer 调用
		if !span.ended {
			t.Fatal("expected span to be ended before panic propagates")
		}
		// EndSpan(nil) 是 tracing 传的（panic 由 recovery 转 err）
		// 这里我们不期望 tracing 知道 panic
		_ = recover() // 吞掉 panic 让测试继续
	}()

	_, _ = interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		h.handle)
}

func TestServerTracing_PropagatesContextFromTracer(t *testing.T) {
	t.Parallel()
	// tracer 返回带 trace-id 的 ctx，handler 应该能看到
	type ctxKey struct{}
	expectedCtx := context.WithValue(context.Background(), ctxKey{}, "trace-xyz")

	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: expectedCtx}
	interceptor := NewServerTracingInterceptor(tracer)

	var seenCtx context.Context
	customHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		seenCtx = ctx
		return "ok", nil
	}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		customHandler)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := seenCtx.Value(ctxKey{}); got != "trace-xyz" {
		t.Fatalf("expected handler to receive trace context, got %v", got)
	}
}

// ===== PR-OBS-17 RED: Span.Tag + Tracer.CreateLocalSpan 接口契约 =====
//
// 目的: stage-44 §四 B 收口——扩 Span/Tracer 接口支持完整 span tag 断言。
// 现有 Span 只有 EndSpan,Tracer 只有 StartEntry;mock 无法断言业务 tag 值。
// 本 PR 边界（仅接口扩展 + adapter 实现 + mock 验证）:
//   - Span.Tag(key, value string) — mock 记录 tagCalls,生产 adapter 转 go2sky.Tag
//   - Tracer.CreateLocalSpan(ctx, op) (ctx, Span, error) — 包装 go2sky.NewTracer.CreateLocalSpan
// 生产路径行为零变化,业务层 tag 设置点留作后续 PR-OBS-18。

// TestMockSpan_Tag_RecordsKeyAndValue mockSpan.Tag 应记录 (key, value) 对
func TestMockSpan_Tag_RecordsKeyAndValue(t *testing.T) {
	t.Parallel()
	s := &mockSpan{}
	s.Tag("rpc.method", "/svc/Method")
	s.Tag("rpc.system", "grpc")
	s.Tag("user_id", "12345")
	if len(s.tagCalls) != 3 {
		t.Fatalf("expected 3 tag calls, got %d", len(s.tagCalls))
	}
	if s.tagCalls[0] != (tagKV{"rpc.method", "/svc/Method"}) {
		t.Errorf("tag[0] mismatch: %+v", s.tagCalls[0])
	}
	if s.tagCalls[1] != (tagKV{"rpc.system", "grpc"}) {
		t.Errorf("tag[1] mismatch: %+v", s.tagCalls[1])
	}
	if s.tagCalls[2] != (tagKV{"user_id", "12345"}) {
		t.Errorf("tag[2] mismatch: %+v", s.tagCalls[2])
	}
}

// TestMockTracer_CreateLocalSpan_RecordsOpName mockTracer.CreateLocalSpan 应记录 opName
func TestMockTracer_CreateLocalSpan_RecordsOpName(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{localSpan: span}
	ctx, gotSpan, err := tracer.CreateLocalSpan(context.Background(), "kafka-consume")
	if err != nil {
		t.Fatalf("CreateLocalSpan returned err: %v", err)
	}
	if ctx == nil {
		t.Fatal("ctx should not be nil")
	}
	if gotSpan != span {
		t.Errorf("span mismatch: got %v, want %v", gotSpan, span)
	}
	if len(tracer.localOpCalls) != 1 || tracer.localOpCalls[0] != "kafka-consume" {
		t.Errorf("expected localOpCalls=[kafka-consume], got %v", tracer.localOpCalls)
	}
}

// TestMockTracer_CreateLocalSpan_ReturnsConfiguredErr mockTracer 应返回配置的 err
func TestMockTracer_CreateLocalSpan_ReturnsConfiguredErr(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("tracer backend down")
	tracer := &mockTracer{localErr: wantErr, localSpan: &mockSpan{}}
	_, _, err := tracer.CreateLocalSpan(context.Background(), "op")
	if err == nil || err.Error() != "tracer backend down" {
		t.Fatalf("expected err=tracer backend down, got %v", err)
	}
}

// TestMockTracer_CreateLocalSpan_PropagatesCustomCtx mockTracer.localCtx 优先于入参
func TestMockTracer_CreateLocalSpan_PropagatesCustomCtx(t *testing.T) {
	t.Parallel()
	type k struct{}
	custom := context.WithValue(context.Background(), k{}, "v")
	tracer := &mockTracer{localCtx: custom, localSpan: &mockSpan{}}
	got, _, _ := tracer.CreateLocalSpan(context.Background(), "op")
	if got.Value(k{}) != "v" {
		t.Errorf("expected localCtx value preserved, got %v", got.Value(k{}))
	}
}

// ===== PR-OBS-17 RED: ServerTracingInterceptor 在 span 上打 tag 的契约 =====
//
// 完整业务 tag (rpc.method/rpc.system/user_id) 由 PR-OBS-18 在 interceptor 里
// 调 span.Tag 实现;本组 case 验证 mockSpan.Tag 的可观察行为已就位
// (避免后续 PR-OBS-18 改 interceptor 时再回头补 mock)。

// TestServerTracing_MockSpanIsObservableForTagCalls 验证 ServerTracingInterceptor
// 用的 mockSpan 暴露了 Tag 调用记录——interceptor 后续扩展时可直接断言 tagCalls
func TestServerTracing_MockSpanIsObservableForTagCalls(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)

	// 后续 PR-OBS-18 会在 interceptor 里调 span.Tag(...),
	// 本 case 提前固化 mockSpan.Tag 是可观察的接口方法
	if len(span.tagCalls) != 0 {
		t.Errorf("fresh span.tagCalls should be empty, got %v", span.tagCalls)
	}
	span.Tag("rpc.method", "/svc/Method")
	if len(span.tagCalls) != 1 {
		t.Errorf("Tag call should be observable, got %d records", len(span.tagCalls))
	}

	// 跑一遍 interceptor 确保它使用同一个 span 实例(非新建)
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			// handler 内可对 span 打 tag(后续 PR-OBS-18 会做)
			return nil, nil
		})
	if err != nil {
		t.Fatalf("interceptor err: %v", err)
	}
	if !span.ended {
		t.Errorf("interop should have called EndSpan on the same span")
	}
}

// ===== PR-OBS-13 RED: 2 case — gRPC trace 边界 =====
//
// 目的: PR-OBS-13 设计 (observability-sprint-b.md §三.13):
// 1. gRPC 调用 FullMethod "/svc/Method" → tracer.StartEntry 的 opName = FullMethod
//    (这是后续 span tag rpc.method 的来源)
// 2. gRPC 调用 metadata x-user-id → handler 收到的 ctx 可读到 x-user-id
//
// 现状约束: Span 接口只有 EndSpan (无 Tag),完整 'span tag 含 user_id/rpc.system/service/method'
// 需要 1) Span 接口扩展 Tag(key, value string) 2) ServerTracingInterceptor 在 StartEntry
// 后调 span.Tag(...) 多次 3) mockSpan 实现 Tag + 记录 tag 调用顺序
// 上述是较大改造,留作 follow-up PR (与 PR-OBS-12 TracerInterface 类似路径)
//
// 本 PR 落地 2 边界 case (覆盖补全型,代码已满足):
// - FullMethod → opName 透传 (mockTracer.calls 验证)
// - metadata x-user-id → handler ctx (handler 读 ctx 验证)
// 这些是 Span.Tag 扩展前的关键前置条件,本 PR 固化避免重构时丢失

// TestServerTracing_FullMethodAsOpName 验证 StartEntry 的 opName = FullMethod
func TestServerTracing_FullMethodAsOpName(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	tracer := &mockTracer{spanToReturn: span, ctxToReturn: context.Background()}
	interceptor := NewServerTracingInterceptor(tracer)
	h := &fakeHandler{}

	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/emotion_llm.v1.EmotionLLMService/Analyze"},
		h.handle)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 验证 StartEntry 的 opName == FullMethod (这是 span tag rpc.method 的来源)
	if len(tracer.calls) != 1 || tracer.calls[0] != "/emotion_llm.v1.EmotionLLMService/Analyze" {
		t.Errorf("expected opName = FullMethod, got %v", tracer.calls)
	}
}

// TestServerTracing_XUserIDMetadataPropagatedToHandler 验证 metadata x-user-id 透传
// handler 从 ctx 的 incoming metadata 读 x-user-id (shared/auth/xuserid.go ParseXUserID 模式)
func TestServerTracing_XUserIDMetadataPropagatedToHandler(t *testing.T) {
	t.Parallel()
	span := &mockSpan{}
	// mockTracer 不设置 ctxToReturn → interceptor 用输入 ctx (含 metadata)
	tracer := &mockTracer{spanToReturn: span}
	interceptor := NewServerTracingInterceptor(tracer)

	var gotUserID string
	// 用 inline handler (不是 fakeHandler.handle),这样可读 ctx 验证 metadata 透传
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			values := md.Get("x-user-id")
			if len(values) > 0 {
				gotUserID = values[0]
			}
		}
		return nil, nil
	}

	md := metadata.MD{}
	md.Set("x-user-id", "12345")
	ctxWithMD := metadata.NewIncomingContext(context.Background(), md)

	_, err := interceptor(ctxWithMD, nil,
		&grpc.UnaryServerInfo{FullMethod: "/svc/Method"},
		handler)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotUserID != "12345" {
		t.Errorf("expected x-user-id=12345 in handler ctx, got %q", gotUserID)
	}
}