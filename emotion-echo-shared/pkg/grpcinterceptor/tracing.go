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

	"github.com/emotion-echo/shared/pkg/logging"
)

// ctxWithTraceIDFromMetadata 从 incoming metadata 的 x-trace-id 取出 trace id
// 并塞进 ctx，使下游 handler 里的 slog.*Context 调用自动带 trace_id 字段。
//
// E2E-21 / E2E-F-13：生产方是 ClientTraceIDInterceptor（shared 侧），
// 源头是 APISIX 注入的 X-Trace-Id（HTTP 入口）。
//
// 无 x-trace-id 时原样返回 ctx（不写入空串）：写入空串会让日志里出现
// "trace_id":"" 这种看着有字段、实则不可查的噪音。
func ctxWithTraceIDFromMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	if v := md.Get("x-trace-id"); len(v) > 0 && v[0] != "" {
		return logging.WithTraceID(ctx, v[0])
	}
	return ctx
}

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
//
// Stage 92 PR-1 新增 CreateExitSpan + CreateEntrySpan:跨进程(Kafka / HTTP)trace
// 透传专用。CreateExitSpan 用于客户端(发请求/发消息),通过 injector 把 sw8 写入
// carrier(Kafka header / HTTP req header);CreateEntrySpan 用于服务端(收消息),
// 通过 extractor 从 carrier 重建父 trace context。
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

	// CreateExitSpan begins an exit span for an outbound operation (e.g. Kafka
	// producer publishing a message). The adapter must call injector(key, value)
	// for each propagation header (key="sw8" / "sw8-correlation"). Returns
	// ctx + span + err; err 非 nil 时调用方应回退到 noop span.
	//
	// operationName: e.g. "kafka-publish"
	// peer:          e.g. "chat-events" (target topic)
	CreateExitSpan(ctx context.Context, operationName, peer string,
		injector func(key, value string) error) (context.Context, Span, error)

	// CreateEntrySpan begins an entry span for an inbound Kafka message,
	// restoring the parent trace from sw8 carried in the message headers.
	// The adapter must call extractor(key) to read each propagation header
	// (key="sw8" / "sw8-correlation"). Returns ctx + span + err; extractor
	// returning ("", nil) is treated as no upstream trace (Valid=false →
	// new trace starts). err 非 nil 时调用方应回退到 noop span.
	//
	// operationName: e.g. "kafka-consume"
	CreateEntrySpan(ctx context.Context, operationName string,
		extractor func(key string) (string, error)) (context.Context, Span, error)
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
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		// E2E-21 / E2E-F-13：先取 trace_id，**放在 tracer==nil 判断之前**。
		// SkyWalking 未启用（tracer=nil）时同样需要 trace_id 进日志，
		// 否则会形成"APM 没开 ⇒ 日志也没 trace_id"的隐蔽耦合。
		ctx = ctxWithTraceIDFromMetadata(ctx)

		if tracer == nil {
			return handler(ctx, req)
		}

		// E2E-26 #3：改用 CreateEntrySpan + incoming metadata extractor ——
		// 从 sw8 提取上游 span 引用（跨进程父子链接），并把承载 span 的 nCtx
		// 交给 handler（下游 exit span 才能续上同一 trace）。
		// 旧实现 StartEntry 不提取 sw8 且返原 ctx ⇒ OAP 实测 RPC 侧 span 全灭、
		// trace 每跳断裂（2026-10-02 运行时证据）。
		md, _ := metadata.FromIncomingContext(ctx)
		extractor := func(key string) (string, error) {
			if md == nil {
				return "", nil
			}
			if vals := md.Get(key); len(vals) > 0 {
				return vals[0], nil
			}
			return "", nil
		}
		nCtx, span, spanErr := tracer.CreateEntrySpan(ctx, info.FullMethod, extractor)
		if spanErr != nil || span == nil {
			// 建 span 失败不阻塞 handler（与 client 端降级语义一致）；
			// trace_id 已在上方注入 ctx，日志链路不受影响。
			return handler(ctx, req)
		}
		ctx = nCtx

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
// Each outbound RPC starts an "exit span" (distributed tracing: 客户端出站 span,
// OAP UI 标记为 exit + peer 信息)。
// If tracer is nil, returns a no-op interceptor.
//
// PR-OBS-19 增强: 与 server 端对称设置 layer/component + 2 个 RPC tag。
// 注: client 端无 user_id(metadata 由上游 server 端注入,client 端无法读到
// 自己的 user_id 除非从 ctx 抽,但 client ctx 通常由调用方注入 — 此处不抽,
// 留作业务层需要时扩展)。
//
// Stage 94 PR-3 §P0-1a 修复：原实现用 `tracer.StartEntry(...)`（语义错 ——
// client 端应创建 exit span 而不是 entry），且完全没有把 sw8 注入 outgoing
// metadata.MD，导致跨 gRPC 进程 trace 链在 BFF→downstream 段完全断裂。
//
// 修复：
//   1) 用 `tracer.CreateExitSpan(ctx, opName, cc.Target(), injector)` 替换 StartEntry
//   2) injector 写入 metadata.MD["sw8"]，把 sw8 透传到 outgoing ctx
//   3) invoker 前 `metadata.NewOutgoingContext(ctx, md)` 把 sw8 挂在 ctx 上 ——
//      这样下游 server 端的 ServerTracingInterceptor 用 CreateEntrySpan 能从
//      incoming metadata 抽到 sw8 重建父 trace
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
		opName := "client:" + method

		// 取 ctx 已有的 outgoing metadata (保留上游链路注入的 x-user-id 等)
		// 防止 md 为 nil (无 outgoing metadata 时 FromOutgoingContext 返 nil,Set 会 panic)
		md, _ := metadata.FromOutgoingContext(ctx)
		if md == nil {
			md = metadata.MD{}
		}

		// peer：cc.Target() 在 cc 为 nil 时 panic（实际 gRPC 调用时 cc 必非 nil）,
		// 测试场景会传 nil,所以这里 nil-guard
		peer := ""
		if cc != nil {
			peer = cc.Target()
		}

		// injector: go2sky/CreateExitSpanWithContext 内部对每个 sw8 header 调一次
		// 我们把值写入 md —— go2sky 完成编码(产出 8 段格式)后回调过来
		injector := func(_, value string) error {
			md.Set("sw8", value)
			return nil
		}

		_, span, err := tracer.CreateExitSpan(ctx, opName, peer, injector)
		if err != nil {
			// CreateExitSpan 失败不阻塞 RPC —— log 后继续(与 server 端降级语义一致)
			// 但仍需走 invoker,否则 RPC 不发
		}
		// CreateExitSpan 后把 md 装回 outgoing ctx —— 让 gRPC 把 sw8 透传给下游 server
		ctx = metadata.NewOutgoingContext(ctx, md)

		if span != nil {
			span.SetSpanLayer(SpanLayerGRPC)
			span.SetComponent(ComponentGoGRPC)
			span.Tag("rpc.system", "grpc")
			span.Tag("rpc.method", method)
		}

		err = invoker(ctx, method, req, reply, cc, opts...)
		if span != nil {
			span.EndSpan(err)
		}
		return err
	}
}