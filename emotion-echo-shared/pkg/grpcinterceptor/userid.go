// Package grpcinterceptor — userid.go
//
// Stage 32 PR-16: gRPC 服务端读 X-User-Id metadata 并注入 ctx。
//
// 与 auth.go（internal API key，svc-to-svc）的区别：
//   - auth.go 验证调用方是哪个 svc（防止 svc A 冒充 svc B）
//   - userid.go 提取 end user id（来自 APISIX jwt-auth 注入的 X-User-Id）
//
// 两者是独立维度，gRPC server 应同时挂载（auth 验证调用方合法，userid 提取业务上下文）。
package grpcinterceptor

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// XUserIDMetadataKey 是 gRPC metadata key for end user id.
//
// MUST be lowercase (gRPC normalizes metadata keys to lowercase).
// 与 HTTP header X-User-Id 对应（APISIX 透传时大小写不敏感）。
const XUserIDMetadataKey = "x-user-id"

// CtxUserIDKeyType 是 context 中 user id 的 key 类型（与 pkg/middleware.CtxUserIDKey 同义，
// 但 gRPC 与 HTTP 中间件是不同包，故单独定义。调用方可用类型断言互转。）
type CtxUserIDKeyType struct{}

// NewServerUserIDInterceptor creates a server-side interceptor that extracts
// end user id from incoming metadata and injects it into ctx.
//
// 行为：
//   - 找到 x-user-id metadata → ParseInt → 注入 ctx（type CtxUserIDKeyType）
//   - 找不到 → 拒绝（codes.Unauthenticated）。生产必须由 APISIX 在调用 gRPC 时透传 metadata。
//   - ParseInt 失败 → 同样拒绝。
//
// 白名单（health check / reflection）：调用方自行判断是否跳过本拦截器（参考 server.go）。
func NewServerUserIDInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		values := md.Get(XUserIDMetadataKey)
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
		}
		uid, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || uid <= 0 {
			return nil, status.Error(codes.Unauthenticated, "invalid x-user-id metadata")
		}
		ctx = context.WithValue(ctx, CtxUserIDKeyType{}, uid)
		return handler(ctx, req)
	}
}

// UserIDFromContext 从 ctx 取 end user id（与 pkg/middleware.UserIDFromContext 同语义，
// 但 key 类型不同——这是 gRPC 与 HTTP 中间件解耦的代价）。
func UserIDFromGRPCContext(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(CtxUserIDKeyType{}).(int64)
	return uid, ok
}
