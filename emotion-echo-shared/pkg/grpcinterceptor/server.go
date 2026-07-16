// Package grpcinterceptor provides gRPC server/client interceptors.
//
// Goals:
//   - Reusable across services (similar to Gin middleware pattern)
//   - No heavy deps (uses google.golang.org/grpc built-in API)
//   - Easy to read; new member can pick up in 30 min
//
// Implemented:
//   - ServerLogging: logs every RPC call (method, peer, latency, error)
//   - ServerRecovery: panic recovery (avoid crashing entire server)
//
// Future (TODO):
//   - ServerTracing: SkyWalking integration
//   - ServerAuth: JWT validation
//   - ClientLogging / ClientTracing
//   - ClientRetry: auto retry transient errors
package grpcinterceptor

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// ServerLoggingInterceptor logs every RPC call.
//
// Output format (one line per call):
//
//	[grpc-server] method=/emotion_llm.v1.EmotionLLMService/Analyze peer=127.0.0.1:54321 latency=42ms code=OK err=<nil>
//	[grpc-server] method=... code=Unavailable err="connection refused"
func ServerLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start)

		peer, _ := peerFromContext(ctx)
		code := "OK"
		if err != nil {
			st, _ := status.FromError(err)
			code = st.Code().String()
		}

		log.Printf(
			"[grpc-server] method=%s peer=%s latency=%dms code=%s err=%v",
			info.FullMethod,
			peer,
			latency.Milliseconds(),
			code,
			err,
		)
		return resp, err
	}
}

// ServerRecoveryInterceptor recovers from panic in handler.
//
// Default behavior: a panic crashes the whole gRPC server (unacceptable).
// With this interceptor, panic in one request is caught, converted to
// INTERNAL status code, server keeps running.
func ServerRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf(
					"[grpc-server] PANIC in %s: %v\n%s",
					info.FullMethod,
					r,
					debug.Stack(),
				)
				// 13 = codes.Internal
				err = status.Errorf(13, "internal error: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}

// peerFromContext extracts client peer (IP:port) from ctx.
// Returns "unknown" on failure (does not affect main flow).
func peerFromContext(ctx context.Context) (string, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "unknown", false
	}
	return p.Addr.String(), true
}