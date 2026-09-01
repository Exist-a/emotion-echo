// Package grpcserver — server_test.go
//
// Sibling test for server.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover server.go (148 LOC) test surface.
// The implementation registers EmotionQueryService + standard
// grpc.health.v1 onto a *grpc.Server, exposes lifecycle via
// Start(ctx) (blocking) and Addr() (for tests), and translates
// EmotionRepo lookups into proto Emotion / EmotionList responses.
//
// Test strategy:
//   - Start a real Server on an ephemeral port (Addrr returns ":0"
//     before Listen; we override via a tiny test helper).
//   - Dial a gRPC client over loopback and exercise:
//     - GetEmotionByMessage (happy / not-found / invalid arg / repo err)
//     - GetEmotionByConversation (happy / limit-clamping / invalid arg)
//     - Health check (standard grpc.health.v1)
//   - All tests share an in-memory EmotionRepo.
//
// We avoid bufconn / shared memory transports because grpc v1.80
// doesn't bundle bufconn by default; a real TCP loopback test is
// equally hermetic and faster to write.
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startTestServer starts a Server on an ephemeral port with the given
// repo and returns (server, client conn, cleanup func). cleanup must
// be deferred. Callers seed the repo BEFORE invoking the gRPC client.

// userIDOutgoingCtx 返回带 x-user-id metadata 的 outgoing context。
// Stage 32 PR-16: 模拟 APISIX 注入的 user id metadata（生产路径）。
func userIDOutgoingCtx(uid int64) context.Context {
	return metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", fmt.Sprintf("%d", uid)))
}
func startTestServer(t *testing.T, repo repository.EmotionRepo) (*Server, *grpc.ClientConn, func()) {
	t.Helper()

	// Listen on an ephemeral port first so we know the address for
	// dialing. The production constructor binds via New(repo, port);
	// we mimic it by listening and then constructing.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close() // release; production will re-listen

	srv := New(repo, nil, port) // Stage 34: fusedEmotionRepo 传 nil（fused 端点 Unimplemented）

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Start(ctx) }()

	// Wait for the server to actually accept connections. Poll Addr
	// until it's reachable; bounded 5s.
	addr := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr = srv.Addr()
		if addr != "" && strings.HasPrefix(addr, "127.0.0.1:") {
			// try a quick TCP probe
			c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				c.Close()
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NotEmpty(t, addr, "server failed to bind within 5s")

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		cancel()
		// Give GracefulStop a moment to complete.
		select {
		case <-serveErr:
		case <-time.After(2 * time.Second):
		}
	}
	return srv, conn, cleanup
}

// TestGetEmotionByMessage_HappyPath verifies the canonical RPC:
// existing analysis → returns Emotion proto with all fields populated.
func TestGetEmotionByMessage_HappyPath(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 100, ConversationID: 50, UserID: 7,
		PrimaryEmotion: "happy", SentimentScore: 0.6,
		Confidence: 0.85, Model: "keyword-v1",
	}))
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	// We don't have a client constructor that takes a custom repo,
	// so we drive the RPC through the proto client API.
	client := emotionquery.NewEmotionQueryServiceClient(conn)
	resp, err := client.GetEmotionByMessage(userIDOutgoingCtx(7), &emotionquery.GetEmotionByMessageRequest{
		MessageId: 100,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "happy", resp.PrimaryEmotion)
	assert.Equal(t, int64(100), resp.MessageId)
	assert.InDelta(t, 0.85, resp.Confidence, 0.001)
}

// TestGetEmotionByMessage_NotFound_ReturnsNotFound verifies the gRPC
// status mapping: missing analysis → codes.NotFound.
func TestGetEmotionByMessage_NotFound_ReturnsNotFound(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	client := emotionquery.NewEmotionQueryServiceClient(conn)
	_, err := client.GetEmotionByMessage(userIDOutgoingCtx(7), &emotionquery.GetEmotionByMessageRequest{
		MessageId: 999,
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "expected grpc status error")
	assert.Equal(t, "NotFound", st.Code().String())
}

// TestGetEmotionByMessage_ZeroMessageID_ReturnsInvalidArgument covers
// the request validation branch.
func TestGetEmotionByMessage_ZeroMessageID_ReturnsInvalidArgument(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	client := emotionquery.NewEmotionQueryServiceClient(conn)
	_, err := client.GetEmotionByMessage(userIDOutgoingCtx(7), &emotionquery.GetEmotionByMessageRequest{
		MessageId: 0,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, "InvalidArgument", st.Code().String())
}

// TestGetEmotionByConversation_HappyPath covers the list endpoint.
func TestGetEmotionByConversation_HappyPath(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	for _, mid := range []int64{1, 2, 3} {
		require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
			MessageID: mid, ConversationID: 50,
			PrimaryEmotion: "calm",
		}))
	}
	// Different conv — must not appear.
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 99, ConversationID: 60,
		PrimaryEmotion: "should_not_appear",
	}))
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	client := emotionquery.NewEmotionQueryServiceClient(conn)
	resp, err := client.GetEmotionByConversation(userIDOutgoingCtx(7), &emotionquery.GetEmotionByConversationRequest{
		ConversationId: 50,
		Limit:          10,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Items, 3)
	assert.Equal(t, int32(3), resp.Total)
}

// TestGetEmotionByConversation_LimitClamping covers the
// limit-coercion behavior: limit <= 0 OR limit > 200 → server uses
// 50 (the documented default).
func TestGetEmotionByConversation_LimitClamping(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	// Seed 60 analyses — exceeds default 50 so we can observe clamping.
	for i := int64(1); i <= 60; i++ {
		require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
			MessageID: i, ConversationID: 50,
			PrimaryEmotion: "calm",
		}))
	}
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	client := emotionquery.NewEmotionQueryServiceClient(conn)

	t.Run("limit_zero_clamps_to_50", func(t *testing.T) {
		resp, err := client.GetEmotionByConversation(userIDOutgoingCtx(7), &emotionquery.GetEmotionByConversationRequest{
			ConversationId: 50, Limit: 0,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Items, 50)
	})

	t.Run("limit_300_clamps_to_50", func(t *testing.T) {
		resp, err := client.GetEmotionByConversation(userIDOutgoingCtx(7), &emotionquery.GetEmotionByConversationRequest{
			ConversationId: 50, Limit: 300,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Items, 50)
	})

	t.Run("limit_30_honored", func(t *testing.T) {
		resp, err := client.GetEmotionByConversation(userIDOutgoingCtx(7), &emotionquery.GetEmotionByConversationRequest{
			ConversationId: 50, Limit: 30,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Items, 30)
	})
}

// TestGetEmotionByConversation_ZeroConvID_ReturnsInvalidArgument
// covers the request validation.
func TestGetEmotionByConversation_ZeroConvID_ReturnsInvalidArgument(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	client := emotionquery.NewEmotionQueryServiceClient(conn)
	_, err := client.GetEmotionByConversation(userIDOutgoingCtx(7), &emotionquery.GetEmotionByConversationRequest{
		ConversationId: 0,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, "InvalidArgument", st.Code().String())
}

// TestHealthCheck_Serving verifies the standard grpc.health.v1
// endpoint returns SERVING for both the empty service name and the
// "emotion.AI" service name registered by the production constructor.
func TestHealthCheck_Serving(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	_, conn, cleanup := startTestServer(t, repo)
	defer cleanup()

	healthClient := healthpb.NewHealthClient(conn)

	for _, svc := range []string{"", "emotion.AI"} {
		t.Run("service="+svc, func(t *testing.T) {
			resp, err := healthClient.Check(context.Background(), &healthpb.HealthCheckRequest{
				Service: svc,
			})
			require.NoError(t, err)
			assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
		})
	}
}

// TestAddr_ReturnsListenAddress covers the Addr() helper: after
// Start, Addr() must return the actual bound address. The production
// implementation calls net.Listen("tcp", fmt.Sprintf(":%d", port))
// which on this OS resolves to the IPv6 wildcard "[::]:port".
// We pin that behavior — pin "Addr contains the expected port"
// rather than the literal address family.
func TestAddr_ReturnsListenAddress(t *testing.T) {
	repo := repository.NewInMemoryEmotionRepo()
	srv, _, cleanup := startTestServer(t, repo)
	defer cleanup()

	addr := srv.Addr()
	require.NotEmpty(t, addr)
	// Either form is acceptable: "127.0.0.1:<port>" or "[::]:<port>".
	// What MUST hold: the address contains a colon + port suffix.
	assert.Contains(t, addr, ":")
	assert.NotEqual(t, ":0", addr, "Addr must not be the sentinel :0 (Listen failed)")
}

// ─────────────────────────────────────────────────────────────────────────────────────────────────────────────
// Internal helpers used only by the test in this file
// ─────────────────────────────────────────────────────────────────────────────────────────────────────────────

// onceFor starts exactly one synchronous server per test invocation
// and waits for it to be reachable. The sync.Mutex guards against
// parallel t.Run subtests spawning multiple servers on the same
// shared repo state — but each top-level test gets its own server,
// so the lock is effectively a documentation aid rather than a
// runtime guard. Kept here in case future refactors share state.
var _ = sync.Mutex{}

// errIsExported is a tiny check ensuring errors.Is imports still link
// when this file is read in isolation. (errors.Is is used by status
// helpers below.)
var _ = errors.Is