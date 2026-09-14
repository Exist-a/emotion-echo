// Package grpcinterceptor 的 client 部分

package grpcinterceptor

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
)

// ClientLoggingInterceptor 记录每次 client RPC 调用
//
// 输出格式：
//
//	[grpc-client] method=/emotion_llm.v1.EmotionLLMService/Analyze target=localhost:50051 latency=42ms err=<nil>
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

		log.Printf(
			"[grpc-client] method=%s target=%s latency=%dms err=%v",
			method,
			cc.Target(),
			latency.Milliseconds(),
			err,
		)
		return err
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

	// 3) logging (always)
	interceptors = append(interceptors, ClientLoggingInterceptor())

	if len(interceptors) == 0 {
		// 全空：返回空 slice（不挂 WithChainUnaryInterceptor,dial 行为不变）
		return nil
	}
	return []grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}
}