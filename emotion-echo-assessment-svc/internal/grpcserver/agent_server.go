// Package grpcserver — agent_server.go
//
// Stage 62 PR-3.3: 实现 AssessmentServiceServer interface（HTTP 与 gRPC 共享 logic 层）
//
// 5 个 rpc 方法对应 assessment-svc 已有的 logic 层：
//   - ListSurveys     → logic.NewListSurveysLogic
//   - GetSurvey       → logic.NewGetSurveyLogic
//   - SubmitSurvey    → logic.NewSubmitSurveyLogic
//   - ListMyResults   → logic.NewGetSurveyResultLogic（复用，PR-3.4 阶段拆）
//   - GetSurveyResult → logic.NewGetSurveyResultLogic

package grpcserver

import (
	"context"
	"errors"
	"fmt"

	"emotion-echo-assessment-svc/internal/logic"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"
	grpcerr "github.com/emotion-echo/shared/pkg/grpcerr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// B4: 注册业务 sentinel errors
func init() {
	grpcerr.MapError(repository.ErrNotFound, codes.NotFound)
}

// assessmentServer 实现 emotionassessment.AssessmentServiceServer
type assessmentServer struct {
	emotionassessment.UnimplementedAssessmentServiceServer
	svcCtx *svc.ServiceContext
}

// toProtoSurveyItem 把 types.SurveyItem 转 proto
func toProtoSurveyItem(t types.SurveyItem) *emotionassessment.SurveyItem {
	return &emotionassessment.SurveyItem{
		Id:            int64(t.ID),
		Code:          t.Code,
		Title:         t.Title,
		Category:      t.Category,
		Version:       int32(t.Version),
		QuestionCount: int32(t.QuestionNum),
	}
}

// toProtoSurvey 把 types.GetSurveyResp 转 proto（含 questions map）
func toProtoSurvey(t *types.GetSurveyResp) *emotionassessment.Survey {
	if t == nil {
		return nil
	}
	questions := make([]*emotionassessment.SurveyQuestion, 0)
	for qid, raw := range t.Questions {
		// 简化：把 raw 转成 map[string]any → SurveyQuestion
		m, _ := raw.(map[string]any)
		q := &emotionassessment.SurveyQuestion{
			Prompt: qid,
		}
		if v, ok := m["id"].(float64); ok {
			q.Id = int64(v)
		}
		if v, ok := m["order"].(float64); ok {
			q.Order = int32(v)
		}
		if v, ok := m["prompt"].(string); ok {
			q.Prompt = v
		}
		if v, ok := m["type"].(string); ok {
			q.QuestionType = v
		}
		if v, ok := m["options"].([]any); ok {
			for _, s := range v {
				if str, ok := s.(string); ok {
					q.Options = append(q.Options, str)
				}
			}
		}
		if v, ok := m["scaleMin"].(float64); ok {
			q.ScaleMin = int32(v)
		}
		if v, ok := m["scaleMax"].(float64); ok {
			q.ScaleMax = int32(v)
		}
		questions = append(questions, q)
	}
	return &emotionassessment.Survey{
		Id:        int64(t.ID),
		Code:      t.Code,
		Title:     t.Title,
		Category:  t.Category,
		Version:   int32(t.Version),
		Questions: questions,
	}
}

// toProtoSurveyResult 把 types.SubmitSurveyResp / GetSurveyResultResp 转 proto
func toProtoSurveyResult(r *types.SubmitSurveyResp) *emotionassessment.SurveyResult {
	if r == nil {
		return nil
	}
	return &emotionassessment.SurveyResult{
		ResultId:   int64(r.ResultID),
		SurveyId:   int64(r.SurveyID),
		TotalScore: int32(r.TotalScore),
		Answered:   int32(r.Answered),
		RiskLevel:  r.RiskLevel,
	}
}

// toProtoSurveyResultItem 把 types.SurveyResultItem 转 proto
func toProtoSurveyResultItem(t types.SurveyResultItem) *emotionassessment.SurveyResult {
	return &emotionassessment.SurveyResult{
		ResultId:   int64(t.ResultID),
		SurveyId:   int64(t.SurveyID),
		RiskLevel:  t.RiskLevel,
	}
}

// ListSurveys 实现 ListSurveys RPC
func (s *assessmentServer) ListSurveys(ctx context.Context, req *emotionassessment.ListSurveysRequest) (*emotionassessment.ListSurveysResponse, error) {
	if s.svcCtx == nil || s.svcCtx.SurveyRepo == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc repository not initialized (degraded start)")
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	resp, err := logic.NewListSurveysLogic(ctx, s.svcCtx).ListSurveys(&types.ListSurveysReq{
		Limit: limit,
	})
	if err != nil {
		return nil, grpcerr.MapToError(err, "listSurveys")
	}
	if resp == nil {
		return &emotionassessment.ListSurveysResponse{Items: nil, Total: 0}, nil
	}
	items := make([]*emotionassessment.SurveyItem, 0, len(resp.Items))
	for _, it := range resp.Items {
		items = append(items, toProtoSurveyItem(it))
	}
	return &emotionassessment.ListSurveysResponse{Items: items, Total: int32(resp.Total)}, nil
}

// GetSurvey 实现 GetSurvey RPC
func (s *assessmentServer) GetSurvey(ctx context.Context, req *emotionassessment.GetSurveyRequest) (*emotionassessment.Survey, error) {
	if s.svcCtx == nil || s.svcCtx.SurveyRepo == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc repository not initialized (degraded start)")
	}
	resp, err := logic.NewGetSurveyLogic(ctx, s.svcCtx).GetSurvey(&types.GetSurveyReq{
		Id: uint64(req.SurveyId),
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "survey not found")
		}
		return nil, grpcerr.MapToError(err, "getSurvey")
	}
	return toProtoSurvey(resp), nil
}

// SubmitSurvey 实现 SubmitSurvey RPC
func (s *assessmentServer) SubmitSurvey(ctx context.Context, req *emotionassessment.SubmitSurveyRequest) (*emotionassessment.SurveyResult, error) {
	if s.svcCtx == nil || s.svcCtx.SurveyRepo == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc repository not initialized (degraded start)")
	}
	// proto Answer oneof → types.Answers map[string]int
	answers := make(map[string]int, len(req.Answers))
	for _, a := range req.Answers {
		key := fmt.Sprintf("%d", a.QuestionId)
		switch v := a.Value.(type) {
		case *emotionassessment.Answer_ScaleValue:
			answers[key] = int(v.ScaleValue)
		case *emotionassessment.Answer_OptionValue:
			answers[key] = int(hashStringToInt(v.OptionValue))
		case *emotionassessment.Answer_TextValue:
			answers[key] = len(v.TextValue) // 文本答案简化为字符数
		}
	}
	resp, err := logic.NewSubmitSurveyLogic(ctx, s.svcCtx).SubmitSurvey(&types.SubmitSurveyReq{
		SurveyId:    uint64(req.SurveyId),
		Answers:     answers,
		DurationSec: int(req.DurationSec),
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "survey not found")
		}
		return nil, grpcerr.MapToError(err, "submitSurvey")
	}
	return toProtoSurveyResult(resp), nil
}

// ListMyResults 实现 ListMyResults RPC（PR-3.4 阶段补全 BFF 端调用）
func (s *assessmentServer) ListMyResults(ctx context.Context, req *emotionassessment.ListMyResultsRequest) (*emotionassessment.ListMyResultsResponse, error) {
	if s.svcCtx == nil || s.svcCtx.SurveyRepo == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc repository not initialized (degraded start)")
	}
	resp, err := logic.NewGetSurveyResultLogic(ctx, s.svcCtx).ListMyResults(&types.ListMyResultsReq{
		Limit: int(req.Limit),
	})
	if err != nil {
		return nil, grpcerr.MapToError(err, "listMyResults")
	}
	if resp == nil {
		return &emotionassessment.ListMyResultsResponse{Items: nil, Total: 0}, nil
	}
	items := make([]*emotionassessment.SurveyResult, 0, len(resp.Items))
	for _, it := range resp.Items {
		items = append(items, toProtoSurveyResultItem(it))
	}
	return &emotionassessment.ListMyResultsResponse{Items: items, Total: int32(resp.Total)}, nil
}

// GetSurveyResult 实现 GetSurveyResult RPC
func (s *assessmentServer) GetSurveyResult(ctx context.Context, req *emotionassessment.GetSurveyResultRequest) (*emotionassessment.SurveyResult, error) {
	if s.svcCtx == nil || s.svcCtx.SurveyRepo == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc repository not initialized (degraded start)")
	}
	resp, err := logic.NewGetSurveyResultLogic(ctx, s.svcCtx).GetSurveyResult(&types.GetSurveyResultReq{
		ResultId: uint64(req.ResultId),
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "result not found")
		}
		return nil, grpcerr.MapToError(err, "getSurveyResult")
	}
	return &emotionassessment.SurveyResult{
		ResultId:   int64(resp.ResultID),
		SurveyId:   int64(resp.SurveyID),
		TotalScore: int32(resp.TotalScore),
		RiskLevel:  resp.RiskLevel,
		DurationSec: int32(resp.DurationSec),
	}, nil
}

// hashStringToInt 字符串 hash 到 int（option value 占位）
func hashStringToInt(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	return h
}