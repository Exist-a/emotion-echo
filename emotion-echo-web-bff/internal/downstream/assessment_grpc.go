// Package downstream — assessment_grpc.go
//
// Stage 62 PR-3.3: BFF → assessment-svc gRPC client（5 RPC 1:1 对应）
//
// E2E-31（2026-10-08，内部 RPC 收敛）：本文件按新 proto 契约重写，并补上两个
// 从未实现的方法——
//   · `ListResults` / `GetResult` 原先是 `return fmt.Errorf("... not implemented ...")`
//     的 stub（兵形实现），导致人格画像只能另建一个 Transport=HTTP 的客户端取数；
//   · 题目不但要保序，还要把 `key`/`title`/`option_items` 还原成前端要的形状
//     （旧实现按 index 重编 `q%d` 且 options 只剩字符串）。
//
// 产出形状必须与 `assessmentHTTPClient` 一致（前端契约，见 assessment.go 的
// normalizeQuestions 注释）——两种 transport 只在取数方式上不同。

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
	out := make([]SurveyItem, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		out = append(out, *fromProtoSurveyItem(item))
	}
	return out, int(resp.GetTotal()), nil
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

// SubmitSurvey gRPC
//
// E2E-31 #3：answers 直传 `map[string]int32`（键 "q1"、值是 option.score），
// 不再走 `Answer{question_id int64}` —— 后者会把 "q1" 数值化、与 scorer 契约冲突。
func (c *assessmentGRPCClient) SubmitSurvey(ctx context.Context, id uint64, req SubmitSurveyReq) (*SubmitSurveyResp, error) {
	cli := emotionassessment.NewAssessmentServiceClient(c.conn)
	answers := make(map[string]int32, len(req.Answers))
	for k, v := range req.Answers {
		answers[k] = int32(v)
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
		ResultID:     uint64(resp.GetResultId()),
		SurveyID:     uint64(resp.GetSurveyId()),
		TotalScore:   resp.GetTotalScore(),
		Answered:     int(resp.GetAnswered()),
		RiskLevel:    resp.GetRiskLevel(),
		FactorScores: resp.GetFactorScores(),
		ScoreKind:    resp.GetScoreKind(),
	}, nil
}

// ListResults gRPC（E2E-31：原为返 error 的 stub，现按 ListMyResults RPC 实现）
func (c *assessmentGRPCClient) ListResults(ctx context.Context, limit int) ([]SurveyResultItem, int, error) {
	cli := emotionassessment.NewAssessmentServiceClient(c.conn)
	if limit <= 0 {
		limit = 20
	}
	resp, err := cli.ListMyResults(withUserID(ctx), &emotionassessment.ListMyResultsRequest{
		Limit: int32(limit),
	})
	if err != nil {
		return nil, 0, wrapGRPCError(err, "assessment listMyResults")
	}
	out := make([]SurveyResultItem, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		out = append(out, *fromProtoSurveyResultItem(item))
	}
	return out, int(resp.GetTotal()), nil
}

// GetResult gRPC（E2E-31：原为返 error 的 stub，现按 GetSurveyResult RPC 实现）
func (c *assessmentGRPCClient) GetResult(ctx context.Context, resultID uint64) (*SurveyResultDetail, error) {
	cli := emotionassessment.NewAssessmentServiceClient(c.conn)
	resp, err := cli.GetSurveyResult(withUserID(ctx), &emotionassessment.GetSurveyResultRequest{
		ResultId: int64(resultID),
	})
	if err != nil {
		return nil, wrapGRPCError(err, "assessment getSurveyResult")
	}
	return fromProtoSurveyResultDetail(resp), nil
}

// ============ proto → types 转换 ============

func fromProtoSurveyItem(item *emotionassessment.SurveyItem) *SurveyItem {
	if item == nil {
		return &SurveyItem{}
	}
	return &SurveyItem{
		ID:          uint64(item.GetId()),
		Code:        item.GetCode(),
		Title:       item.GetTitle(),
		Description: item.GetDescription(),
		Category:    item.GetCategory(),
		QuestionNum: int(item.GetQuestionCount()),
		Version:     int(item.GetVersion()),
	}
}

// fromProtoSurvey 产出与 HTTP 路径**同形状**的 questions（有序数组 + 结构化选项）
//
// E2E-31 #1/#4：旧实现有两个硬伤——
//   ① `questions[fmt.Sprintf("q%d", i)]` 按**切片下标**重编号，而 proto 的切片顺序
//      来自服务端 map 遍历（无序）⇒ 键名与真实 "q1" 不保证对应；
//   ② `"options": q.Options`（`[]string`）⇒ 前端 `opt.id` / `opt.score` 全取不到。
// 现在直接用 proto 的 `key`/`title`/`option_items`，无需再猜。
func fromProtoSurvey(s *emotionassessment.Survey) *SurveyDetail {
	if s == nil {
		return nil
	}
	questions := make([]map[string]any, 0, len(s.GetQuestions()))
	for _, q := range s.GetQuestions() {
		key := q.GetKey()
		if key == "" {
			key = fmt.Sprintf("q%d", q.GetOrder())
		}
		title := q.GetTitle()
		if title == "" {
			title = q.GetPrompt() // 兼容老服务端（只填 prompt）
		}
		opts := make([]map[string]any, 0, len(q.GetOptionItems()))
		for _, o := range q.GetOptionItems() {
			opts = append(opts, map[string]any{
				"id":    int(o.GetId()),
				"text":  o.GetText(),
				"score": int(o.GetScore()),
			})
		}
		questions = append(questions, map[string]any{
			"id":      key,
			"title":   title,
			"type":    q.GetQuestionType(),
			"options": opts,
		})
	}
	return &SurveyDetail{
		ID:          uint64(s.GetId()),
		Code:        s.GetCode(),
		Title:       s.GetTitle(),
		Description: s.GetDescription(),
		Category:    s.GetCategory(),
		Version:     int(s.GetVersion()),
		Questions:   questions,
	}
}

func fromProtoSurveyResultItem(r *emotionassessment.SurveyResult) *SurveyResultItem {
	if r == nil {
		return &SurveyResultItem{}
	}
	return &SurveyResultItem{
		ResultID:     uint64(r.GetResultId()),
		SurveyID:     uint64(r.GetSurveyId()),
		TotalScore:   r.GetTotalScore(),
		RiskLevel:    r.GetRiskLevel(),
		SubmittedAt:  r.GetSubmittedAt(),
		FactorScores: r.GetFactorScores(),
		ScoreKind:    r.GetScoreKind(),
	}
}

func fromProtoSurveyResultDetail(r *emotionassessment.SurveyResult) *SurveyResultDetail {
	if r == nil {
		return nil
	}
	answers := make(map[string]any, len(r.GetAnswers()))
	for k, v := range r.GetAnswers() {
		answers[k] = int(v)
	}
	return &SurveyResultDetail{
		ResultID:     uint64(r.GetResultId()),
		SurveyID:     uint64(r.GetSurveyId()),
		UserID:       r.GetUserId(),
		TotalScore:   r.GetTotalScore(),
		RiskLevel:    r.GetRiskLevel(),
		DurationSec:  int(r.GetDurationSec()),
		Answers:      answers,
		SubmittedAt:  r.GetSubmittedAt(),
		FactorScores: r.GetFactorScores(),
		ScoreKind:    r.GetScoreKind(),
	}
}
