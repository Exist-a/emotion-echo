package outbox

import (
	"context"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== CL-03 Phase 3：relay 用 outbox 行里存的 sw8 恢复父 ctx =====
//
// 现状（建档实测）：relay 跑在后台根 ctx（main.go 的 relayCtx = WithCancel(Background())）
// ⇒ kafka_publisher 的 CreateExitSpan 建的是「根」span，一条消息的 trace 断在
// outbox→relay 处。修法：relay 发布前用 `trace_sw8` 列恢复父 ctx（CreateEntrySpan +
// extractor），让这一跳接回原始请求的 trace。

// relayTraceCtxKey 是假 tracer 往返回的 ctx 里塞的标记，用来判断 publisher 收到的是
// 「恢复后的 ctx」还是「原来的后台根 ctx」。
type relayTraceCtxKey struct{}

// recordingTracer 记录 CreateEntrySpan 的调用参数，返回带标记的 ctx + 可断言的 span。
type recordingTracer struct {
	entryOpCalls  []string
	extractedSw8  string
	spanEndCalls  int
	lastEndErr    error
	returnsNoSpan bool
}

func (t *recordingTracer) StartEntry(ctx context.Context, _ string) (context.Context, grpcinterceptor.Span) {
	return ctx, &relayTestSpan{}
}

func (t *recordingTracer) CreateLocalSpan(ctx context.Context, _ string) (context.Context, grpcinterceptor.Span, error) {
	return ctx, &relayTestSpan{}, nil
}

func (t *recordingTracer) CreateExitSpan(ctx context.Context, _, _ string, injector func(string, string) error) (context.Context, grpcinterceptor.Span, error) {
	if injector != nil {
		_ = injector("sw8", "mock-outbound-sw8")
	}
	return ctx, &relayTestSpan{}, nil
}

func (t *recordingTracer) CreateEntrySpan(ctx context.Context, opName string, extractor func(string) (string, error)) (context.Context, grpcinterceptor.Span, error) {
	t.entryOpCalls = append(t.entryOpCalls, opName)
	if extractor != nil {
		if v, _ := extractor(grpcinterceptor.SW8HeaderName); v != "" {
			t.extractedSw8 = v
		}
	}
	if t.returnsNoSpan {
		return ctx, nil, nil
	}
	// 返回带标记的 ctx —— 模拟"span 承载 ctx"（生产投产 adapter 行为）
	return context.WithValue(ctx, relayTraceCtxKey{}, true), &relayTestSpan{owner: t}, nil
}

func (t *recordingTracer) EncodeSW8(context.Context, string) (string, error) { return "", nil }

// relayTestSpan 记录 EndSpan（含 err 透传）。
type relayTestSpan struct{ owner *recordingTracer }

func (s *relayTestSpan) EndSpan(err error) {
	if s.owner != nil {
		s.owner.spanEndCalls++
		s.owner.lastEndErr = err
	}
}
func (s *relayTestSpan) Tag(string, string) {}
func (s *relayTestSpan) SetSpanLayer(int32) {}
func (s *relayTestSpan) SetComponent(int32) {}

// ctxCapturingPublisher 记录每次 Publish 收到的 ctx，便于断言"publisher 看到的是恢复后的 ctx"。
type ctxCapturingPublisher struct {
	gotCtxs   []context.Context
	publishFn func(ctx context.Context, topic string, e *events.Event) error
}

func (p *ctxCapturingPublisher) Publish(ctx context.Context, topic string, e *events.Event) error {
	p.gotCtxs = append(p.gotCtxs, ctx)
	if p.publishFn != nil {
		return p.publishFn(ctx, topic, e)
	}
	return nil
}

func (p *ctxCapturingPublisher) Close() error { return nil }

func mustCreateOutboxRow(t *testing.T, repo repository.OutboxRepo, evid, sw8 string) {
	t.Helper()
	require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
		EventID:   evid,
		EventType: "message.created",
		Topic:     "chat-events",
		Payload: []byte(`{"id":"` + evid + `","type":"message.created","source":"chat-svc",` +
			`"time":"2026-08-30T10:00:00Z","data":{"messageId":1,"conversationId":1,"userId":1,` +
			`"role":"user","content":"hi","createdAt":1}}`),
		TraceSW8: sw8,
	}))
}

// TestRelay_FlushOnce_RestoresParentCtxFromTraceSW8Column ★ Phase 3 核心 ★
//
// 行里有 trace_sw8 ⇒ relay 必须：① 用它调 CreateEntrySpan(op="outbox-relay")
// ② 把**恢复后的 ctx** 交给 publisher ③ 发布后 EndSpan（err 透传）。
func TestRelay_FlushOnce_RestoresParentCtxFromTraceSW8Column(t *testing.T) {
	const storedSW8 = "1-YWJj-1-2-0-chat-svc-inst-POST /api/v1/messages-1.2.3.4:80"

	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-trace-1", storedSW8)

	tr := &recordingTracer{}
	pub := &ctxCapturingPublisher{}
	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	r.Tracer = tr

	require.NoError(t, r.FlushOnce(context.Background()))

	assert.Equal(t, []string{"outbox-relay"}, tr.entryOpCalls,
		"有 trace_sw8 时必须调 CreateEntrySpan 恢复父 ctx（operationName=outbox-relay）")
	assert.Equal(t, storedSW8, tr.extractedSw8,
		"extractor 必须把列里存的 sw8 原样交给 tracer")
	require.Len(t, pub.gotCtxs, 1)
	assert.Equal(t, true, pub.gotCtxs[0].Value(relayTraceCtxKey{}),
		"publisher 必须收到**恢复后的 ctx**（否则 trace 仍断在 outbox→relay）")
	assert.Equal(t, 1, tr.spanEndCalls, "恢复用的 span 必须 EndSpan（否则 duration 失真）")
	assert.NoError(t, tr.lastEndErr, "发布成功时 EndSpan 应收到 nil err")
}

// TestRelay_FlushOnce_EmptyTraceSW8_DoesNotCreateSpan 向后兼容：
// 历史行 / tracer=nil 时列是空的 ⇒ 不建 span、行为与改前一致。
func TestRelay_FlushOnce_EmptyTraceSW8_DoesNotCreateSpan(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-trace-2", "")

	tr := &recordingTracer{}
	pub := &ctxCapturingPublisher{}
	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	r.Tracer = tr

	require.NoError(t, r.FlushOnce(context.Background()))

	assert.Empty(t, tr.entryOpCalls, "空 trace_sw8 不得建 span")
	require.Len(t, pub.gotCtxs, 1)
	assert.Nil(t, pub.gotCtxs[0].Value(relayTraceCtxKey{}),
		"空列时 publisher 应拿到原 ctx（行为与改前逐字一致）")
	assert.Equal(t, 0, tr.spanEndCalls)
}

// TestRelay_FlushOnce_PublishErrorPassedToSpanEndSpan 发布失败时 err 必须透传给
// EndSpan，让 OAP 把这一跳标红（否则失败被 trace 掩盖）。
func TestRelay_FlushOnce_PublishErrorPassedToSpanEndSpan(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-trace-3", "1-ccc-1-2-0-svc-inst-ep-1.1.1.1:80")

	tr := &recordingTracer{}
	pub := &ctxCapturingPublisher{publishFn: func(context.Context, string, *events.Event) error {
		return assert.AnError
	}}
	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	r.Tracer = tr

	require.NoError(t, r.FlushOnce(context.Background()))

	assert.Equal(t, 1, tr.spanEndCalls)
	assert.ErrorIs(t, tr.lastEndErr, assert.AnError, "发布失败必须透传到 EndSpan(err)")
}

// TestRelay_FlushOnce_NoInjectedTracer_FallsBackToGlobalAndPublishes
// 未注入 Tracer 时回落读全局（CL-F-14 语义：tracer 可能后台才补挂）。
// 全局为 nil 时**必须照常发布**（trace 降级不得影响投递语义）。
func TestRelay_FlushOnce_NoInjectedTracer_FallsBackToGlobalAndPublishes(t *testing.T) {
	prev := grpcinterceptor.GlobalTracer()
	grpcinterceptor.SetGlobalTracer(nil)
	t.Cleanup(func() { grpcinterceptor.SetGlobalTracer(prev) })

	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-trace-4", "1-ddd-1-2-0-svc-inst-ep-1.1.1.1:80")

	pub := &ctxCapturingPublisher{}
	r := NewRelay(repo, pub, 100*time.Millisecond, 10) // 不设 Tracer

	require.NoError(t, r.FlushOnce(context.Background()))
	assert.Len(t, pub.gotCtxs, 1, "tracer 缺失时仍必须发布（trace 降级不影响投递）")

	pending, _ := repo.ListPending(context.Background(), 10)
	assert.Empty(t, pending, "行应被 MarkSent")
}

// TestRelay_FlushOnce_TracerReturnsNilSpan_FallsBackToOriginalCtx
// tracer 存在但建 span 失败（返 nil span）⇒ 退回原 ctx 发布，不得 panic。
func TestRelay_FlushOnce_TracerReturnsNilSpan_FallsBackToOriginalCtx(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-trace-5", "1-eee-1-2-0-svc-inst-ep-1.1.1.1:80")

	tr := &recordingTracer{returnsNoSpan: true}
	pub := &ctxCapturingPublisher{}
	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	r.Tracer = tr

	require.NoError(t, r.FlushOnce(context.Background()))
	require.Len(t, pub.gotCtxs, 1)
	assert.Nil(t, pub.gotCtxs[0].Value(relayTraceCtxKey{}), "建 span 失败应退回原 ctx")
}

// 编译期断言：假 tracer 满足 shared 的 Tracer 接口
var _ grpcinterceptor.Tracer = (*recordingTracer)(nil)
