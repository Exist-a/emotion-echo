package grpcserver

// CL-F-66：ai-svc gRPC 8892 越权面 —— 服务端必须校验 x-internal-api-key。
//
// 背景：compose 把 8892 发布到宿主（0.0.0.0），而拦截器链只有 user-id/tracing/logging/recovery
// ⇒ 任意本机进程可冒他人 user_id 读写情绪与记忆。修法复用 shared/pkg/grpcinterceptor 的
// NewServerAuthInterceptor（llm-service 同款语义），并沿用 service-aware 白名单跳过 health。

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"emotion-echo-ai-svc/internal/repository"
)

// TestServerAuth_MissingKey_Rejected 未带 key ⇒ UNAUTHENTICATED（而不是放行后落库）。
func TestServerAuth_MissingKey_Rejected(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServerWithOptions(t, repo, WithInternalAPIKey("secret-key"))
	defer cleanup()

	cli := emotionquery.NewEmotionQueryServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// 带 user id 但不带 key：排除"其实是 user-id 拦截器拒的"这种误判（CL-F-70 同型教训）
	ctx = metadata.AppendToOutgoingContext(ctx, "x-user-id", "1")

	_, err := cli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 999})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err), "无 key 必须被拒: %v", err)
	require.Contains(t, status.Convert(err).Message(), "api key",
		"错误必须来自 internal-key 拦截器（missing api key），不是 user-id")
}

// TestServerAuth_WithKey_Allowed 带正确 key ⇒ 放行（业务码不是 UNAUTHENTICATED）。
func TestServerAuth_WithKey_Allowed(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServerWithOptions(t, repo, WithInternalAPIKey("secret-key"))
	defer cleanup()

	cli := emotionquery.NewEmotionQueryServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-internal-api-key", "secret-key",
		"x-user-id", "1", // 业务 RPC 同时要求 user id（既有拦截器），与鉴权正交
	)

	_, err := cli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 999})
	require.NotEqual(t, codes.Unauthenticated, status.Code(err), "带正确 key 不应被拒: %v", err)
}

// TestServerAuth_HealthWithoutKey_Allowed health probe 不带 key（k8s/docker healthcheck）。
func TestServerAuth_HealthWithoutKey_Allowed(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServerWithOptions(t, repo, WithInternalAPIKey("secret-key"))
	defer cleanup()

	hc := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := hc.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err, "health 必须免鉴权")
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
}

// TestServerAuth_DisabledWhenKeyEmpty 未配置 key ⇒ 保持旧行为（本地/测试/未下发 env 的环境不被打断）。
func TestServerAuth_DisabledWhenKeyEmpty(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServerWithOptions(t, repo, WithInternalAPIKey(""))
	defer cleanup()

	cli := emotionquery.NewEmotionQueryServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// 只带 user id（不带 key）：用于证明"未配 key 时是旧行为，不是被 auth 拦下"
	ctx = metadata.AppendToOutgoingContext(ctx, "x-user-id", "1")

	_, err := cli.GetEmotionByMessage(ctx, &emotionquery.GetEmotionByMessageRequest{MessageId: 999})
	require.NotEqual(t, codes.Unauthenticated, status.Code(err), "未配 key 时应放行: %v", err)
}
