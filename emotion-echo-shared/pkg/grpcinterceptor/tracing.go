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
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// OAP SpanLayer enum 值(对应 skywalking.apache.org/repo/.../SpanLayer)。
// 与 grpcinterceptor 共享避免重复声明 — 测试可断言精确值。
const (
	SpanLayerHTTP  int32 = 2
	SpanLayerGRPC  int32 = 5
	SpanLayerMQ    int32 = 6 // Kafka 沿用 MQ layer
	SpanLayerCache int32 = 7
	SpanLayerDB    int32 = 3
)

// OAP Component ID(对应 skywalking component-libraries.yml)。
// 5001=Go gRPC, 5002=Go HTTP, 5003=Go Kafka 等。
const (
	ComponentGoGRPC    int32 = 5001
	ComponentGoHTTP    int32 = 5002
	ComponentGoKafka   int32 = 5003
	ComponentGoRedis   int32 = 5008
	ComponentGoPostgre int32 = 5005
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
//
// PR-OBS-19 增强: 在 span 上设置 layer/component + 3 个标准 tag:
//
//   - SetSpanLayer(GRPC=5): 标记 span 为 GRPC layer,OAP UI 按 layer 维度过滤
//   - SetComponent(Go gRPC=5001): 标记 client library,OAP UI 按 component 维度过滤
//   - Tag(rpc.system, "grpc"): OpenTelemetry 风格 RPC 系统标识
//   - Tag(rpc.method, info.FullMethod): OpenTelemetry 风格 RPC 方法(如 /svc/Method)
//   - Tag(user_id, <metadata x-user-id>): 用户维度(APISIX jwt-auth 注入)
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

		// PR-OBS-19: 设置 OAP layer/component + 3 个 RPC tag
		span.SetSpanLayer(SpanLayerGRPC)
		span.SetComponent(ComponentGoGRPC)
		span.Tag("rpc.system", "grpc")
		span.Tag("rpc.method", info.FullMethod)
		if uid := extractUserIDFromCtx(ctx); uid != "" {
			span.Tag("user_id", uid)
		}

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

// extractUserIDFromCtx 从 gRPC incoming metadata 抽 x-user-id header。
// 与 emotion-echo-ai-svc/internal/grpcserver/server.go:newServiceAwareUserIDInterceptor
// 解析模式一致(metadata.FromIncomingContext → md.Get("x-user-id"))。
// 与 GinAuthMiddleware 解析 X-User-Id header 模式不同(gRPC metadata vs HTTP header)。
//
// 返回 "" 表示无 user_id(middleware 不打 tag)。
// 注:此函数不依赖 shared/pkg/auth,纯 metadata 操作;若未来 auth helper 统一,
// 可改为 shared/pkg/auth/ExtractUserIDFromGRPCContext() 复用。
func extractUserIDFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-user-id")
	if len(values) == 0 {
		return ""
	}
	uid := values[0]
	// 防御性校验: 与 GinAuthMiddleware strconv.ParseInt 校验一致,确保 OAP tag 数字合法
	if _, err := strconv.ParseInt(uid, 10, 64); err != nil {
		return ""
	}
	return uid
}

// NewClientTracingInterceptor creates a client-side tracing interceptor.
//
// Each outbound RPC starts a "client span" (exit span in distributed tracing terms).
// If tracer is nil, returns a no-op interceptor.
//
// PR-OBS-19 增强: 与 server 端对称设置 layer/component + 2 个 RPC tag。
// 注: client 端无 user_id(metadata 由上游 server 端注入,client 端无法读到
// 自己的 user_id 除非从 ctx 抽,但 client ctx 通常由调用方注入 — 此处不抽,
// 留作业务层需要时扩展)。
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
		span.SetSpanLayer(SpanLayerGRPC)
		span.SetComponent(ComponentGoGRPC)
		span.Tag("rpc.system", "grpc")
		span.Tag("rpc.method", method)
		err := invoker(ctx, method, req, reply, cc, opts...)
		span.EndSpan(err)
		return err
	}
}