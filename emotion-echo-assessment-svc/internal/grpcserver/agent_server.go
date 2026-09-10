// Package grpcserver — agent_server.go
//
// Stage 62 PR-3.2: 实现 AssessmentServiceServer interface
//
// 5 个 rpc 方法对应 assessment-svc 已有的 logic 层（PR-3.3 阶段补完）：
//   - ListSurveys / GetSurvey / SubmitSurvey
//   - ListMyResults / GetSurveyResult
//
// PR-3.2 阶段：所有方法占位返 Unimplemented（与 chat-svc PR-GRPC-2 / user-svc PR-3.2 同步节奏）。

package grpcserver

import (
	"context"

	"emotion-echo-assessment-svc/internal/svc"

	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// assessmentServer 实现 emotionassessment.AssessmentServiceServer
type assessmentServer struct {
	emotionassessment.UnimplementedAssessmentServiceServer
	svcCtx *svc.ServiceContext
}

// ListSurveys 占位实现
func (s *assessmentServer) ListSurveys(ctx context.Context, req *emotionassessment.ListSurveysRequest) (*emotionassessment.ListSurveysResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "ListSurveys: PR-3.3 阶段补完")
}

// GetSurvey 占位实现
func (s *assessmentServer) GetSurvey(ctx context.Context, req *emotionassessment.GetSurveyRequest) (*emotionassessment.Survey, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "GetSurvey: PR-3.3 阶段补完")
}

// SubmitSurvey 占位实现
func (s *assessmentServer) SubmitSurvey(ctx context.Context, req *emotionassessment.SubmitSurveyRequest) (*emotionassessment.SurveyResult, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "SubmitSurvey: PR-3.3 阶段补完")
}

// ListMyResults 占位实现
func (s *assessmentServer) ListMyResults(ctx context.Context, req *emotionassessment.ListMyResultsRequest) (*emotionassessment.ListMyResultsResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "ListMyResults: PR-3.3 阶段补完")
}

// GetSurveyResult 占位实现
func (s *assessmentServer) GetSurveyResult(ctx context.Context, req *emotionassessment.GetSurveyResultRequest) (*emotionassessment.SurveyResult, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "GetSurveyResult: PR-3.3 阶段补完")
}