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
type Span interface {
	EndSpan(err error)
}

// Tracer is the minimal interface needed by ServerTracingInterceptor.
//
// go2sky/opentelemetry adapters implement this interface.
type Tracer interface {
	// StartEntry begins an entry span for an incoming request.
	// Returns ctx (with span attached) and the span itself.
	//
	// operationName: e.g. gRPC method "/emotion_llm.v1.EmotionLLMService/Analyze"
	StartEntry(ctx context.Context, operationName string) (context.Context, Span)
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