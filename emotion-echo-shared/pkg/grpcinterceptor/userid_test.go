package grpcinterceptor

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// fakeServer 用 bufconn 测试 gRPC interceptor（无需真实 TCP）
func newBufConnServer(t *testing.T) (grpc.UnaryServerInterceptor, *bufconn.Listener, func()) {
	lis := bufconn.Listen(1024 * 64)
	var capturedUID int64
	var capturedOK bool
	uidInterceptor := NewServerUserIDInterceptor()
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		uidInterceptor,
		func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			capturedUID, capturedOK = UserIDFromGRPCContext(ctx)
			return "OK", nil
		},
	))
	go func() {
		_ = srv.Serve(lis)
	}()
	return uidInterceptor, lis, func() {
		srv.Stop()
		_ = lis.Close()
		_ = capturedUID
		_ = capturedOK
	}
}

func TestServerUserIDInterceptor_AcceptsValidMetadata(t *testing.T) {
	_, lis, cleanup := newBufConnServer(t)
	defer cleanup()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 用 metadata.NewOutgoingContext 注入 x-user-id
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "1001"))
	// 直接调一个 dummy method（通过自定义 UnaryInvoker 不行，gRPC 必须有 service 注册）
	// 简化为：仅测试 interceptor function 直接调用
	interceptor := NewServerUserIDInterceptor()
	md := metadata.New(map[string]string{"x-user-id": "1001"})
	ctx2 := metadata.NewIncomingContext(context.Background(), md)
	_, err = interceptor(ctx2, nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			uid, ok := UserIDFromGRPCContext(ctx)
			if !ok {
				t.Fatalf("expected user id in ctx")
			}
			if uid != 1001 {
				t.Fatalf("want uid=1001 got %d", uid)
			}
			return "OK", nil
		})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	_ = ctx
}

func TestServerUserIDInterceptor_RejectsMissingMetadata(t *testing.T) {
	interceptor := NewServerUserIDInterceptor()
	ctx := context.Background()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			t.Fatalf("handler should not be called")
			return nil, nil
		})
	if err == nil {
		t.Fatalf("expected Unauthenticated error")
	}
}

func TestServerUserIDInterceptor_RejectsInvalidUID(t *testing.T) {
	interceptor := NewServerUserIDInterceptor()
	md := metadata.New(map[string]string{"x-user-id": "abc"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			t.Fatalf("handler should not be called")
			return nil, nil
		})
	if err == nil {
		t.Fatalf("expected Unauthenticated error")
	}
}

func TestServerUserIDInterceptor_RejectsZeroUID(t *testing.T) {
	interceptor := NewServerUserIDInterceptor()
	md := metadata.New(map[string]string{"x-user-id": "0"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			t.Fatalf("handler should not be called")
			return nil, nil
		})
	if err == nil {
		t.Fatalf("expected Unauthenticated error for zero uid")
	}
}
