// Package grpcserver — metric_server_nildeps_test.go
//
// Stage 77 RED：与 user-svc 同构的 nil-repo 契约（stage-76 §二.3）。
// openPostgres 失败时 main.go 带 EventRepo=nil 的 ServiceContext 启动（日志原话
// "EventRepo = nil"），报表 gRPC 端点必须返 codes.Unavailable，绝不 panic。
package grpcserver

import (
	"context"
	"testing"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"emotion-echo-analytics-svc/internal/svc"
)

func TestAnalyticsServer_ReportsDaily_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &analyticsServer{svcCtx: &svc.ServiceContext{}}
	_, err := s.ReportsDaily(context.Background(), &emotionanalytics.ReportsDailyRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "repo 未注入应返 Unavailable 而非 panic")
}

func TestAnalyticsServer_MentalHealthHistory_NilRepo_ReturnsUnavailable(t *testing.T) {
	s := &analyticsServer{svcCtx: &svc.ServiceContext{}}
	_, err := s.MentalHealthHistory(context.Background(), &emotionanalytics.MentalHealthHistoryRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}
