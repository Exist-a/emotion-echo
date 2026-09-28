// Package grpcinterceptor 的 client 部分

package grpcinterceptor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/emotion-echo/shared/pkg/logging"
)

// ClientLoggingInterceptor 记录每次 client RPC 调用
//
// 输出格式（字面量不可改：client_test.go:170 与 deploy/apisix/test_jwt_auth_runtime.sh
// 都依赖 "[grpc-client] method=" 这个前缀）：
//
//	[grpc-client] method=/emotion_llm.v1.EmotionLLMService/Analyze target=localhost:50051 latency=42ms err=<nil>
//
// E2E-21 / E2E-F-13：改用带 ctx 的 slog 调用。
// 原因：logging.enrichHandler 是**从 ctx** 取 trace_id 再注入日志字段的
// （logging.go:126-139），原先用 log.Printf（无 ctx）⇒ 即便上游把 trace_id
// 塞进了 ctx，这行日志的 trace_id 也恒为空。实测表现为"header 已注入、日志里没有"。
func ClientLoggingInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		latency := time.Since(start)

		target := ""
		if cc != nil { // 测试会传 nil，直接取会 panic
			target = cc.Target()
		}
		slog.InfoContext(ctx, fmt.Sprintf(
			"[grpc-client] method=%s target=%s latency=%dms err=%v",
			method,
			target,
			latency.Milliseconds(),
			err,
		))
		return err
	}
}

// ClientTraceIDInterceptor 把 ctx 里的 trace id 写进 outgoing metadata。
//
// E2E-21 / E2E-F-13：这是 gRPC 侧 trace 链路的**唯一生产点**。
// HTTP 侧由 APISIX 注入 X-Trace-Id 并被 gin 中间件塞进 ctx；跨进程到下游 svc 时，
// gRPC 不传 HTTP header ⇒ 必须借 metadata 透传，否则 BFF 有 trace_id、下游恒空。
//
// 与 x-user-id 透传同构（见 userid.go / NewClientTracingInterceptor 的 outgoing md 处理）。
// 两条硬约束：
//   - ctx 里没有 trace id 时**不注入**（不得伪造 —— 伪造出来的 ID 查不到任何链路，
//     比恒空更危险：运维会以为链路 ID 丢了而放弃排查）
//   - 只追加不覆盖（sw8 / x-user-id 必须原样保留）
func ClientTraceIDInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		tid := logging.TraceIDFromCtx(ctx)
		if tid == "" {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		// FromOutgoingContext 在无 outgoing md 时返 nil，直接 Set 会 panic
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.MD{}
		} else {
			md = md.Copy() // 不改调用方持有的 map
		}
		md.Set("x-trace-id", tid)
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientTimeoutInterceptor 给每次 client 调用加超时
//
// 不指定 context deadline 时使用 defaultTimeout，避免永久阻塞
func ClientTimeoutInterceptor(defaultTimeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 如果 ctx 已经有 deadline，跳过
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientDialOptions 组装 svc-to-svc 默认 gRPC client dial options。
//
// 链顺序（与 ai-svc analyzer grpc_analyzer.go:70-77 同模式）：
//   1) tracing    —— tracer 非 nil 时挂 CreateExitSpan + sw8 metadata 透传
//   2) timeout    —— defaultTimeout > 0 时挂"无 deadline 自动加 timeout"
//   3) logging    —— 始终挂,记录每次 RPC latency/err
//   4) traceid    —— 始终挂（E2E-21），把 ctx 里的 trace id 透传给下游
//
// 返回 []grpc.DialOption 供 grpc.NewClient(addr, opts...) 链入。
//
// 参数：
//   tracer:         可选 SkyWalking Tracer（PR-OBS-17 接口）；nil 时不挂 tracing
//   defaultTimeout: 可选统一超时；<=0 时不挂 timeout（让业务层 ctx 控制）
//
// Stage 94 PR-4 §P0-1b：code-review-2026-09-14 §P0-1 主修。
// 用法示例（BFF）：
//
//	return dialGRPC(addr, name, sharedgrpc.ClientDialOptions(tracer, 5*time.Second)...)
func ClientDialOptions(tracer Tracer, defaultTimeout time.Duration) []grpc.DialOption {
	var interceptors []grpc.UnaryClientInterceptor

	// 1) tracing
	if tracer != nil {
		interceptors = append(interceptors, NewClientTracingInterceptor(tracer))
	}

	// 2) timeout
	if defaultTimeout > 0 {
		interceptors = append(interceptors, ClientTimeoutInterceptor(defaultTimeout))
	}

	// 3) traceid (always, E2E-21)
	//
	// 必须排在 logging **之前**：grpc 的 interceptor 链是外层先入，
	// logging 紧贴 invoker 最先执行调用；traceid 只需在 invoker 前改好 ctx，
	// 因此放在 logging 之前（更外层）不影响 logging 自身读到 ctx。
	interceptors = append(interceptors, ClientTraceIDInterceptor())

	// 4) logging (always)
	interceptors = append(interceptors, ClientLoggingInterceptor())

	if len(interceptors) == 0 {
		// 全空：返回空 slice（不挂 WithChainUnaryInterceptor,dial 行为不变）
		return nil
	}
	return []grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}
}