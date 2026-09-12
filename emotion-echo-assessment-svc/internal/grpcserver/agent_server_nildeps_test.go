// Package grpcserver — agent_server_nildeps_test.go
//
// Stage 77 RED：与 user-svc 同构的 nil-repo 契约（stage-76 §二.3）。
// openPostgres 失败时 main.go 带 SurveyRepo=nil 的 ServiceContext 启动，
// gRPC 端点必须返 codes.Unavailable，绝不 panic。
package grpcserver

import (
	"context"
	"testing"

	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"emotion-echo-assessment-svc/internal/svc"
)

func TestAssessmentServer_ListSurveys_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &assessmentServer{svcCtx: &svc.ServiceContext{}}
	_, err := s.ListSurveys(context.Background(), &emotionassessment.ListSurveysRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "repo 未注入应返 Unavailable 而非 panic")
}

func TestAssessmentServer_SubmitSurvey_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &assessmentServer{svcCtx: &svc.ServiceContext{}}
	_, err := s.SubmitSurvey(context.Background(), &emotionassessment.SubmitSurveyRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}
