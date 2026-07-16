package grpcinterceptor

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// =====================================================
// NewServerAuthInterceptor 测试
// =====================================================

func TestServerAuth_EmptyKey_AllowsAll(t *testing.T) {
	t.Parallel()
	interceptor := NewServerAuthInterceptor("")
	h := &fakeHandler{}

	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle,
	)
	if err != nil {
		t.Fatalf("expected no error when auth disabled, got: %v", err)
	}
	if h.called != 1 {
		t.Fatalf("expected handler called, got %d", h.called)
	}
}

func TestServerAuth_MissingMetadata_Rejects(t *testing.T) {
	t.Parallel()
	interceptor := NewServerAuthInterceptor("secret-key")
	h := &fakeHandler{}

	_, err := interceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/Test/Method"},
		h.handle,
	)
	if err == nil {
		t.Fatal("expected UNAUTHENTICATED error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", st.Code())
	}
	if h.called != 0 {
		t.Fatalf("handler should not be called, got %d", h.called)
	}
}

func TestServerAuth_WrongKey_Rejects(t *testing.T) {
	t.Parallel()
	interceptor := NewServerAuthInterceptor("expected-key")
	h := &fakeHandler{}

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{
			"x-internal-api-key": "wrong-key",
		}))

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/Test/Method"}, h.handle)
	if err == nil {
		t.Fatal("expected UNAUTHENTICATED for wrong key")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", st.Code())
	}
	if !containsStr(err.Error(), "invalid api key") {
		t.Fatalf("expected 'invalid api key' in err, got: %v", err)
	}
}

func TestServerAuth_CorrectKey_Allows(t *testing.T) {
	t.Parallel()
	interceptor := NewServerAuthInterceptor("expected-key")
	h := &fakeHandler{}

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{
			"x-internal-api-key": "expected-key",
		}))

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/Test/Method"}, h.handle)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok-response" {
		t.Fatalf("expected ok-response, got %v", resp)
	}
	if h.called != 1 {
		t.Fatalf("expected handler called once, got %d", h.called)
	}
}

// =====================================================
// WithInternalAPIKey 测试
// =====================================================

func TestWithInternalAPIKey_EmptyKey_NoChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got := WithInternalAPIKey(ctx, "")

	// 空 key 不修改 ctx
	md, ok := metadata.FromOutgoingContext(got)
	if ok && len(md.Get(InternalAPIKeyMetadataKey)) > 0 {
		t.Fatal("expected no metadata when key empty")
	}
}

func TestWithInternalAPIKey_NonEmpty_SetsMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got := WithInternalAPIKey(ctx, "secret-123")

	md, ok := metadata.FromOutgoingContext(got)
	if !ok {
		t.Fatal("expected metadata in outgoing context")
	}
	keys := md.Get(InternalAPIKeyMetadataKey)
	if len(keys) != 1 || keys[0] != "secret-123" {
		t.Fatalf("expected key 'secret-123', got %v", keys)
	}
}

func TestWithInternalAPIKey_PreservesExistingMetadata(t *testing.T) {
	t.Parallel()
	md := metadata.New(map[string]string{"x-trace-id": "abc123"})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	got := WithInternalAPIKey(ctx, "key1")
	md2, _ := metadata.FromOutgoingContext(got)
	if len(md2.Get("x-trace-id")) == 0 {
		t.Fatal("existing metadata lost")
	}
	if len(md2.Get(InternalAPIKeyMetadataKey)) == 0 {
		t.Fatal("new api key not set")
	}
}

// =====================================================
// 端到端 round-trip：WithInternalAPIKey + ServerAuth
// =====================================================

func TestServerAuth_EndToEnd_RoundTrip(t *testing.T) {
	t.Parallel()
	expectedKey := "round-trip-key"
	server := NewServerAuthInterceptor(expectedKey)
	h := &fakeHandler{}

	// 模拟 client：构造 outgoing metadata
	clientCtx := WithInternalAPIKey(context.Background(), expectedKey)

	// 模拟 client→server：outgoing metadata 转为 incoming metadata
	md, _ := metadata.FromOutgoingContext(clientCtx)
	serverCtx := metadata.NewIncomingContext(context.Background(), md)

	resp, err := server(serverCtx, nil, &grpc.UnaryServerInfo{FullMethod: "/Test/Method"}, h.handle)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if resp != "ok-response" {
		t.Fatalf("expected ok-response, got %v", resp)
	}
}

// =====================================================
// 工具
// =====================================================

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ensure errors pkg imported for future use
var _ = errors.New