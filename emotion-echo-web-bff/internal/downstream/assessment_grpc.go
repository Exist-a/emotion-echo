// Package downstream — assessment_grpc.go
//
// Stage 62 PR-3.3: BFF → assessment-svc gRPC client（5 RPC 1:1 对应）

package downstream

import (
	"context"
	"fmt"

	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"

	"google.golang.org/grpc"
)

// assessmentGRPCClient 是 AssessmentClient 的 gRPC 实现
type assessmentGRPCClient struct {
	conn *grpc.ClientConn
}

// NewAssessmentGRPCClient 构造（conn=nil → 返 nil）
func NewAssessmentGRPCClient(conn *grpc.ClientConn) AssessmentClient {
	if conn == nil {
		return nil
	}
	return &assessmentGRPCClient{conn: conn}
}

// ListSurveys gRPC
func (c *assessmentGRPCClient) ListSurveys(ctx context.Context, limit int) ([]SurveyItem, int, error) {
	cli := emotionassessment.NewAssessmentServiceClient(c.conn)
	if limit <= 0 {
		limit = 20
	}
	resp, err := cli.ListSurveys(withUserID(ctx), &emotionassessment.ListSurveysRequest{
		Limit: int32(limit),
	})
	if err != nil {
		return nil, 0, wrapGRPCError(err, "assessment listSurveys")
	}
	out := make([]SurveyItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, *fromProtoSurveyItem(item))
	}
	return out, int(resp.Total), nil
}

// GetSurvey gRPC
func (c *assessmentGRPCClient) GetSurvey(ctx context.Context, id uint64) (*SurveyDetail, error) {
	cli := emotionassessment.NewAssessmentServiceClient(c.conn)
	resp, err := cli.GetSurvey(withUserID(ctx), &emotionassessment.GetSurveyRequest{
		SurveyId: int64(id),
	})
	if err != nil {
		return nil, wrapGRPCError(err, "assessment getSurvey")
	}
	return fromProtoSurvey(resp), nil
}

// SubmitSurvey gRPC（proto Answer oneof → types map）
func (c *assessmentGRPCClient) SubmitSurvey(ctx context.Context, id uint64, req SubmitSurveyReq) (*SubmitSurveyResp, error) {
	cli := emotionassessment.NewAssessmentServiceClient(c.conn)
	answers := make([]*emotionassessment.Answer, 0, len(req.Answers))
	for qid, score := range req.Answers {
		qidInt := parseInt64(qid)
		scoreInt := int32(score)
		answers = append(answers, &emotionassessment.Answer{
			QuestionId: qidInt,
			Value:      &emotionassessment.Answer_ScaleValue{ScaleValue: scoreInt},
		})
	}
	resp, err := cli.SubmitSurvey(withUserID(ctx), &emotionassessment.SubmitSurveyRequest{
		SurveyId:    int64(id),
		Answers:     answers,
		DurationSec: int32(req.DurationSec),
	})
	if err != nil {
		return nil, wrapGRPCError(err, "assessment submitSurvey")
	}
	return &SubmitSurveyResp{
		ResultID:   uint64(resp.ResultId),
		SurveyID:   uint64(resp.SurveyId),
		TotalScore: float64(resp.TotalScore),
		Answered:   int(resp.Answered),
		RiskLevel:  resp.RiskLevel,
	}, nil
}

// ListResults gRPC（暂无对应 RPC，PR-3.3 阶段返空）
//
// 注：proto emotionassessment.AssessmentService 暂未包含 ListMyResults 的 gRPC 实现
// （实际上 5 RPC 都包含，但 ListMyResults 字段命名需对照；本批只覆盖
// ListSurveys/GetSurvey/SubmitSurvey，ListResults/GetResult 留 PR-3.4）
func (c *assessmentGRPCClient) ListResults(ctx context.Context, limit int) ([]SurveyResultItem, int, error) {
	return nil, 0, fmt.Errorf("downstream: assessment ListResults gRPC not implemented in PR-3.3 (todo)")
}

// GetResult gRPC（PR-3.4 实现）
func (c *assessmentGRPCClient) GetResult(ctx context.Context, resultID uint64) (*SurveyResultDetail, error) {
	return nil, fmt.Errorf("downstream: assessment GetResult gRPC not implemented in PR-3.3 (todo)")
}

// ============ proto → types 转换 ============

func fromProtoSurveyItem(item *emotionassessment.SurveyItem) *SurveyItem {
	return &SurveyItem{
		ID:          uint64(item.Id),
		Code:        item.Code,
		Title:       item.Title,
		Category:    item.Category,
		QuestionNum: int(item.QuestionCount),
		Version:     int(item.Version),
	}
}

func fromProtoSurvey(s *emotionassessment.Survey) *SurveyDetail {
	if s == nil {
		return nil
	}
	questions := make(map[string]any, len(s.Questions))
	for i, q := range s.Questions {
		questions[fmt.Sprintf("q%d", i)] = map[string]any{
			"id":       uint64(q.Id),
			"order":    int(q.Order),
			"prompt":   q.Prompt,
			"type":     q.QuestionType,
			"options":  q.Options,
			"scaleMin": int(q.ScaleMin),
			"scaleMax": int(q.ScaleMax),
		}
	}
	return &SurveyDetail{
		ID:        uint64(s.Id),
		Code:      s.Code,
		Title:     s.Title,
		Category:  s.Category,
		Version:   int(s.Version),
		Questions: questions,
	}
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}