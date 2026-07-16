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