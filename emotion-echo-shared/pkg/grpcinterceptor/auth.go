// Package grpcinterceptor auth-related interceptors
//
// Stage 12: ServerAuth + ClientMetadata helpers for internal API key auth.
//
// Design:
//   - Server checks metadata "x-internal-api-key" against expected value
//   - Client (Go) sets the same metadata via WithInternalAPIKey(ctx, key)
//   - If server's expectedKey is empty → auth is DISABLED (dev mode)
//   - If expectedKey is non-empty → UNAUTHENTICATED if missing/wrong
//
// Why internal API key, not JWT?
//   - Internal svc-to-svc calls don't carry user JWT (JWT is for end users)
//   - mTLS is more secure but harder to deploy (cert management)
//   - API key via metadata is simple + effective for internal trust
//
// Future (TODO):
//   - mTLS (production-grade)
//   - per-service keys (identify caller svc)
//   - rotate keys without downtime
package grpcinterceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// InternalAPIKeyMetadataKey is the gRPC metadata key for internal API key.
//
// MUST be lowercase (gRPC normalizes metadata keys to lowercase).
const InternalAPIKeyMetadataKey = "x-internal-api-key"

// NewServerAuthInterceptor creates a server-side auth interceptor.
//
// expectedKey:
//   - non-empty: enforce auth (UNAUTHENTICATED if missing/wrong)
//   - empty: auth disabled (dev mode only)
func NewServerAuthInterceptor(expectedKey string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Auth disabled when no expected key configured
		if expectedKey == "" {
			return handler(ctx, req)
		}

		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		keys := md.Get(InternalAPIKeyMetadataKey)
		if len(keys) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing api key")
		}
		if keys[0] != expectedKey {
			return nil, status.Error(codes.Unauthenticated, "invalid api key")
		}

		return handler(ctx, req)
	}
}

// WithInternalAPIKey adds the API key to outgoing metadata.
//
// Usage:
//
//	ctx := grpcinterceptor.WithInternalAPIKey(context.Background(), "secret")
//	resp, err := client.Analyze(ctx, req)
//
// If ctx already has outgoing metadata (e.g., trace-id), this appends
// without overwriting.
func WithInternalAPIKey(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		return ctx
	}
	// AppendToOutgoingContext preserves existing metadata (unlike NewOutgoingContext which replaces)
	return metadata.AppendToOutgoingContext(ctx,
		InternalAPIKeyMetadataKey, apiKey,
	)
}