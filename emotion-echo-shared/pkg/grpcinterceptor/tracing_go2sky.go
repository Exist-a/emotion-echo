// Package grpcinterceptor · go2sky adapter for ServerTracing
//
// Stage 13 production adapter: bridges go2sky.Tracer to our minimal Tracer interface.
//
// Why split into two files?
//   - tracing.go: pure interface + interceptor, no third-party deps, easy to test
//   - tracing_go2sky.go: depends on github.com/SkyAPM/go2sky
//   - This package remains "light" (no go2sky in test scope) when not needed
//
// Note: go2sky v1.5.0 only exposes CreateLocalSpan / CreateExitSpan, no
// dedicated EntrySpan. For gRPC server tracing, CreateLocalSpan is the
// recommended pattern. See:
// https://github.com/SkyAPM/go2sky/issues/118

package grpcinterceptor

import (
	"context"
	"time"

	"github.com/SkyAPM/go2sky"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

// Go2SkySpan wraps go2sky's span so it satisfies our Span interface.
type Go2SkySpan struct {
	span go2sky.Span
}

// EndSpan finishes the go2sky span. err indicates success/failure.
func (s *Go2SkySpan) EndSpan(err error) {
	if s.span == nil {
		return
	}
	if err != nil {
		s.span.Error(time.Now(), err.Error())
	}
	s.span.End()
}

// Tag adds a key/value attribute to the underlying go2sky span.
//
// PR-OBS-17: Span 接口扩展,本方法实现 key/value → go2sky.Tag(key) 转译。
// go2sky v1.5 Tag 类型为 string (type Tag string),无运行时转换开销。
// nil 内部 span 时为 no-op(避免 panic 路径污染调用方)。
func (s *Go2SkySpan) Tag(key, value string) {
	if s.span == nil {
		return
	}
	s.span.Tag(go2sky.Tag(key), value)
}

// SetSpanLayer PR-OBS-19 扩展:设置 span layer (OAP enum int32)。
// layer 值约定(对应 agentv3.SpanLayer enum):
//   2 = HTTP, 5 = GRPC, 6 = MQ (kafka 暂无,沿用 MQ)
//
// 实现:cast int32 → agentv3.SpanLayer 后调用底层 span.SetSpanLayer。
// nil 内部 span 时 no-op。
func (s *Go2SkySpan) SetSpanLayer(layer int32) {
	if s.span == nil {
		return
	}
	s.span.SetSpanLayer(agentv3.SpanLayer(layer))
}

// SetComponent PR-OBS-19 扩展:设置 span component ID。
// 常用 component ID:
//   5001 = Go gRPC server / client
//   5002 = Go HTTP server
//   5003 = Go Kafka consumer / producer
//
// 实现:直接传 int32 给底层 span.SetComponent(go2sky 已接受 int32)。
// nil 内部 span 时 no-op。
func (s *Go2SkySpan) SetComponent(componentID int32) {
	if s.span == nil {
		return
	}
	s.span.SetComponent(componentID)
}

// Go2SkyTracer adapts go2sky.Tracer to our minimal Tracer interface.
type Go2SkyTracer struct {
	tracer *go2sky.Tracer
}

// NewGo2SkyTracer wraps an existing go2sky tracer.
func NewGo2SkyTracer(tracer *go2sky.Tracer) *Go2SkyTracer {
	if tracer == nil {
		return nil
	}
	return &Go2SkyTracer{tracer: tracer}
}

// StartEntry implements Tracer.
//
// E2E-26 #3 修正：改用 go2sky CreateEntrySpan（空 extractor = 无上游引用的
// 根 entry span），并返回**承载 span 的 nCtx**。
//
// 旧实现（Stage 46 时代）用 CreateExitSpanWithContext 空注入 + 返回原 ctx，
// 注释称 "go2sky v1.5 doesn't expose a dedicated EntrySpan" —— 该陈述失实
// （同文件 CreateEntrySpan 即为 v1.5 的 EntrySpan 封装，Kafka consumer 一直在用）。
// 旧实现的两个后果（2026-10-02 OAP 运行时实证）：
//  1. span type=Exit 而非 Entry —— UI 语义错位；
//  2. 返回原 ctx ⇒ gin handler 的下游 exit span 续不上入口，每跳各起新 trace。
//
// 无上游引用是安全的：propagation.decode 对空 sw8 直接 return nil（不报错），
// go2sky 视为 root entry。gRPC server 端的 sw8 提取走 CreateEntrySpan
// （NewServerTracingInterceptor），不经本方法。
func (t *Go2SkyTracer) StartEntry(ctx context.Context, operationName string) (context.Context, Span) {
	if t == nil || t.tracer == nil {
		return ctx, &Go2SkySpan{} // no-op span
	}
	span, nCtx, err := t.tracer.CreateEntrySpan(ctx, operationName,
		func(string) (string, error) { return "", nil })
	if err != nil || span == nil {
		return ctx, &Go2SkySpan{}
	}
	return nCtx, &Go2SkySpan{span: span}
}

// CreateLocalSpan implements Tracer for local (non-network) operations
// such as Kafka consumer message handling or cron jobs.
//
// PR-OBS-17 新增: 包装 go2sky.NewTracer.CreateLocalSpan + WithOperationName。
// 返回 (ctx, Span, error);nil receiver / nil tracer 时降级 noop span + nil err,
// 保持与 StartEntry 一致的容错语义(调用方无需 nil 检查)。
func (t *Go2SkyTracer) CreateLocalSpan(ctx context.Context, operationName string) (context.Context, Span, error) {
	if t == nil || t.tracer == nil {
		return ctx, &Go2SkySpan{}, nil
	}
	span, nCtx, err := t.tracer.CreateLocalSpan(ctx, go2sky.WithOperationName(operationName))
	if err != nil {
		return ctx, &Go2SkySpan{}, err
	}
	if span == nil {
		return ctx, &Go2SkySpan{}, nil
	}
	return nCtx, &Go2SkySpan{span: span}, nil
}

// CreateExitSpan implements Tracer for outbound operations that propagate
// the sw8 trace header to a downstream peer (e.g. Kafka producer publishing
// a message to chat-events topic).
//
// Stage 92 PR-1: 包装 go2sky.Tracer.CreateExitSpanWithContext,通过注入器把
// go2sky 内部的 SpanContext 编码为 sw8 + sw8-correlation string,调用方写到
// carrier(Kafka RecordHeader / HTTP req header)。
//
// injector 必须实现 propagation.Injector 协议:对每个 (key, value) 调一次;
// 当前实现只注入 sw8(sw8-correlation 通常为空,go2sky 自动跳过)。
//
// 返回 (ctx, Span, error);nil receiver / nil tracer 时降级 noop span + nil err。
func (t *Go2SkyTracer) CreateExitSpan(
	ctx context.Context, operationName, peer string,
	injector func(key, value string) error,
) (context.Context, Span, error) {
	if t == nil || t.tracer == nil || injector == nil {
		return ctx, &Go2SkySpan{}, nil
	}
	span, nCtx, err := t.tracer.CreateExitSpanWithContext(ctx, operationName, peer,
		func(key, value string) error { return injector(key, value) })
	if err != nil {
		return nCtx, &Go2SkySpan{}, err
	}
	if span == nil {
		return nCtx, &Go2SkySpan{}, nil
	}
	return nCtx, &Go2SkySpan{span: span}, nil
}

// CreateEntrySpan implements Tracer for inbound operations that restore the
// parent trace from a sw8 header carried by the upstream peer (e.g. Kafka
// consumer reading a message that chat-svc producer wrote sw8 into).
//
// Stage 92 PR-1: 包装 go2sky.Tracer.CreateEntrySpan,通过 extractor 从 carrier
// 抽 sw8 + sw8-correlation,内部解码为 SpanContext 并 attach 为父 trace。
//
// extractor 必须实现 propagation.Extractor 协议:对每个 key 返回对应的 value
// (找不到返 "", nil 表示无此 header,go2sky 内部判 Valid=false → 新 trace 起点)。
//
// 返回 (ctx, Span, error);nil receiver / nil tracer / nil extractor 时降级 noop。
func (t *Go2SkyTracer) CreateEntrySpan(
	ctx context.Context, operationName string,
	extractor func(key string) (string, error),
) (context.Context, Span, error) {
	if t == nil || t.tracer == nil || extractor == nil {
		return ctx, &Go2SkySpan{}, nil
	}
	span, nCtx, err := t.tracer.CreateEntrySpan(ctx, operationName,
		func(key string) (string, error) { return extractor(key) })
	if err != nil {
		return nCtx, &Go2SkySpan{}, err
	}
	if span == nil {
		return nCtx, &Go2SkySpan{}, nil
	}
	return nCtx, &Go2SkySpan{span: span}, nil
}