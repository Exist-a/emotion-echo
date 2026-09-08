// Package grpcinterceptor tracing-related interceptors
//
// Stage 13: ServerTracingInterceptor wraps every gRPC call in a trace span.
//
// Design:
//   - Tracer is an interface (dependency injection)
//   - No hard dependency on go2sky/opentelemetry in this package
//   - Tests use mock Tracer to assert span lifecycle
//   - Production uses go2sky adapter (see tracing_go2sky.go)

package grpcinterceptor

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

// Span represents an active trace span. EndSpan finishes the span and reports errors.
//
// Tag adds a key/value attribute to the span, surfaced in trace UIs as filterable
// dimensions (rpc.method / user_id / messaging.kafka.topic etc.). PR-OBS-17 扩接口
// 用于 stage-44 §四 B 收口;具体业务 tag 在调用方(SkywalkingMiddleware / interceptor /
// ConsumerGroupHandler)调 Tag 设置,本接口不感知具体 tag key 集合。
//
// SetSpanLayer / SetComponent PR-OBS-19 扩展:
//   - 用于区分 span 类型 (HTTP / GRPC / Kafka 等),让 OAP UI 过滤"按 layer 维度"
//   - SpanLayer 用 int32 而非 go2sky 原生 SpanLayer 枚举,避免 shared 包依赖
//     agentv3 protobuf;adapter 内部转 go2sky 原生 enum
//   - nil receiver / nil 内部 span 时为 no-op
type Span interface {
	EndSpan(err error)
	// Tag 在 span 上设置一个属性。
	// key/value 均为字符串(避免 mock 端依赖 go2sky.Tag 类型);
	// adapter 内部负责类型转换(如转 go2sky.Tag)。
	Tag(key, value string)
	// SetSpanLayer 设置 span layer (HTTP=2 / GRPC=5 等 OAP 枚举值)。
	// PR-OBS-19: OAP UI 按 layer 维度过滤。
	SetSpanLayer(layer int32)
	// SetComponent 设置 span component ID (Go gRPC component=5001 等)。
	// PR-OBS-19: OAP UI 按 component 维度过滤。
	SetComponent(componentID int32)
}

// Tracer is the minimal interface needed by ServerTracingInterceptor.
//
// go2sky/opentelemetry adapters implement this interface.
//
// PR-OBS-17 新增 CreateLocalSpan 用于 Kafka consumer 等本地操作的 span 创建;
// 原 StartEntry 专用于入口 span(server side)。两方法在生产 adapter 中均映射到
// go2sky.Tracer 的对应方法。
type Tracer interface {
	// StartEntry begins an entry span for an incoming request.
	// Returns ctx (with span attached) and the span itself.
	//
	// operationName: e.g. gRPC method "/emotion_llm.v1.EmotionLLMService/Analyze"
	StartEntry(ctx context.Context, operationName string) (context.Context, Span)

	// CreateLocalSpan begins a local span (no remote peer), e.g. for a Kafka
	// consumer message handler or a cron job. Returns ctx + span + err.
	// err 非 nil 时调用方应回退到 noop span(与 StartEntry 行为一致)。
	CreateLocalSpan(ctx context.Context, operationName string) (context.Context, Span, error)
}

// NewServerTracingInterceptor creates a server-side tracing interceptor.
//
// If tracer is nil, returns a no-op interceptor (tracing disabled).
// This allows services to enable tracing conditionally via config.
func NewServerTracingInterceptor(tracer Tracer) grpc.UnaryServerInterceptor {
	if tracer == nil {
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		ctx, span := tracer.StartEntry(ctx, info.FullMethod)

		// Defer span.EndSpan to ensure span is always finished,
		// including on panic. This makes trace data complete.
		defer func() {
			if r := recover(); r != nil {
				// panic detected: mark span as failed, then re-throw
				err = fmt.Errorf("panic: %v", r)
				span.EndSpan(err)
				panic(r)
			}
			span.EndSpan(err)
		}()

		return handler(ctx, req)
	}
}

// NewClientTracingInterceptor creates a client-side tracing interceptor.
//
// Each outbound RPC starts a "client span" (exit span in distributed tracing terms).
// If tracer is nil, returns a no-op interceptor.
func NewClientTracingInterceptor(tracer Tracer) grpc.UnaryClientInterceptor {
	if tracer == nil {
		return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}

	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, span := tracer.StartEntry(ctx, "client:"+method)
		err := invoker(ctx, method, req, reply, cc, opts...)
		span.EndSpan(err)
		return err
	}
}