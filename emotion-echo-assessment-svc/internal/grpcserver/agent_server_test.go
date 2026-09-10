// Package grpcserver — agent_server_test.go
//
// Stage 62 PR-3.2: assessment-svc gRPC server 实现单元测试（仿 user_server_test.go）
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startTestServer 在临时端口启 gRPC server
func startAssessmentTestServer(t *testing.T) (*Server, *grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	srv := New(nil, port)
	require.NotNil(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Start(ctx) }()

	addr := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr = srv.Addr()
		if addr != "" && addr != fmt.Sprintf(":%d", port) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		cancel()
		select {
		case <-serveErr:
		case <-time.After(2 * time.Second):
		}
	}
	return srv, conn, cleanup
}

// ============ Unimplemented 阶段行为契约 ============

func TestAssessmentServer_ListSurveys_MissingUserID_ReturnsNonOK(t *testing.T) {
	_, conn, cleanup := startAssessmentTestServer(t)
	defer cleanup()

	client := emotionassessment.NewAssessmentServiceClient(conn)
	_, err := client.ListSurveys(context.Background(), &emotionassessment.ListSurveysRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.NotEqual(t, "OK", st.Code().String(), "期望被拦截（非 OK）")
}

func TestAssessmentServer_GetSurvey_WithUserID_NoPanic(t *testing.T) {
	_, conn, cleanup := startAssessmentTestServer(t)
	defer cleanup()

	client := emotionassessment.NewAssessmentServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	_, err := client.GetSurvey(ctx, &emotionassessment.GetSurveyRequest{SurveyId: 1})
	if err == nil {
		t.Fatal("PR-3.2 阶段 GetSurvey 应返 Unavailable（svcCtx 未注入）")
	}
	st, _ := status.FromError(err)
	t.Logf("GetSurvey code=%s msg=%s", st.Code(), st.Message())
}

func TestAssessmentServer_SubmitSurvey_RequiresUserID(t *testing.T) {
	_, conn, cleanup := startAssessmentTestServer(t)
	defer cleanup()

	client := emotionassessment.NewAssessmentServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	_, err := client.SubmitSurvey(ctx, &emotionassessment.SubmitSurveyRequest{
		SurveyId:    1,
		Answers:     nil,
		DurationSec: 60,
	})
	if err == nil {
		t.Fatal("PR-3.2 阶段 SubmitSurvey 应返 Unavailable（svcCtx 未注入）")
	}
	st, _ := status.FromError(err)
	assert.Equal(t, "Unavailable", st.Code().String(), "PR-3.2 阶段期望 Unavailable（svcCtx 未注入）")
}

// ============ 单元方法：type / New 存在性 ============

func TestAssessmentServer_TypeImplement(t *testing.T) {
	var _ emotionassessment.AssessmentServiceServer = (*assessmentServer)(nil)
}

func TestAssessmentServer_New_ReturnsNonNil(t *testing.T) {
	srv := New(nil, 8886)
	require.NotNil(t, srv)
	assert.Equal(t, 8886, srv.port)
}

func TestAssessmentServer_PortPropagated(t *testing.T) {
	srv := New(nil, 9092)
	assert.Equal(t, 9092, srv.port)
}

// ============ Message types ============

func TestAssessmentServer_MessageTypesExist(t *testing.T) {
	assert.NotNil(t, &emotionassessment.Survey{})
	assert.NotNil(t, &emotionassessment.SurveyResult{})
	assert.NotNil(t, &emotionassessment.Answer{})
}