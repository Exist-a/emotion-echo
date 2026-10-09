// Package grpcserver — agent_server.go
//
// Stage 62 PR-3.3: 实现 AssessmentServiceServer interface（HTTP 与 gRPC 共享 logic 层）
//
// 5 个 rpc 方法对应 assessment-svc 已有的 logic 层：
//   - ListSurveys     → logic.NewListSurveysLogic
//   - GetSurvey       → logic.NewGetSurveyLogic
//   - SubmitSurvey    → logic.NewSubmitSurveyLogic
//   - ListMyResults   → logic.NewGetSurveyResultLogic
//   - GetSurveyResult → logic.NewGetSurveyResultLogic
//
// E2E-31（2026-10-08，内部 RPC 收敛）：本文件的**转换层**按新 proto 契约重写——
//   · 选项改为结构化 `SurveyOption`（`option_items`），题干取 JSONB 的 `title`
//   · 题目按 JSONB 键（"q1"…"qN"）**有序**输出，且 `key` 保真
//     （旧实现 `for qid, raw := range` 遍历 map **无序**，BFF 侧再按 index 重编 `q%d`
//     ⇒ 顺序与键名都不可靠）
//   · 作答直接复用 `map<string,int32>`（旧实现把 `"q1"` 数值化成 `"1"`，与 scorer 契约冲突）
//   · 结果补 `factor_scores` / `score_kind` / `answers` / `submitted_at`，`total_score`
//     改 double 不再截断（DB 侧本就是 float64）

package grpcserver

import (
	"context"
	"errors"
	"sort"

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

// ============ JSONB → proto 转换辅助 ============

// toFloat 从 JSONB 解出的 any 取数值（encoding/json 默认给 float64）
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// toStr 从 JSONB 解出的 any 取字符串
func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// toProtoOptionItems 把 JSONB 的 `options: [{id,text,score}]` 转结构化选项（E2E-31 #1）
func toProtoOptionItems(raw any) []*emotionassessment.SurveyOption {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]*emotionassessment.SurveyOption, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, &emotionassessment.SurveyOption{
			Id:    int32(toFloat(m["id"])),
			Text:  toStr(m["text"]),
			Score: int32(toFloat(m["score"])),
		})
	}
	return out
}

// trailingNumber 取字符串尾部数字（"q12" → 12；无数字 → 0），用于题目排序
func trailingNumber(s string) int {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	n := 0
	for _, c := range s[i:] {
		n = n*10 + int(c-'0')
	}
	return n
}

// sortedQuestionKeys 返回按尾部数字升序的问题键（"q1" < "q2" < ... < "q10"）
func sortedQuestionKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, nj := trailingNumber(keys[i]), trailingNumber(keys[j])
		if ni != nj {
			return ni < nj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// toProtoAnswers 把 JSONB 的 answers（map[string]any，值多为数字）转 map[string]int32
func toProtoAnswers(raw map[string]any) map[string]int32 {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]int32, len(raw))
	for k, v := range raw {
		out[k] = int32(toFloat(v))
	}
	return out
}

// toProtoSurveyItem 把 types.SurveyItem 转 proto（含 description，E2E-31 #2）
func toProtoSurveyItem(t types.SurveyItem) *emotionassessment.SurveyItem {
	return &emotionassessment.SurveyItem{
		Id:            int64(t.ID),
		Code:          t.Code,
		Title:         t.Title,
		Category:      t.Category,
		Version:       int32(t.Version),
		QuestionCount: int32(t.QuestionNum),
		Description:   t.Description,
	}
}

// toProtoSurvey 把 types.GetSurveyResp 转 proto（题目按 key 有序 + 结构化选项）
//
// E2E-31 #1/#4：旧实现有两个硬伤——
//   ① `for qid, raw := range t.Questions` 遍历 map **无序**，且把 qid 塞进了 `prompt`；
//   ② options 只认 `[]string`，而 JSONB 是 `[{id,text,score}]` ⇒ 选项结构整块丢失。
// 现在：按尾部数字排序，`key`/`title` 保真，选项转 `SurveyOption`。
func toProtoSurvey(t *types.GetSurveyResp) *emotionassessment.Survey {
	if t == nil {
		return nil
	}
	keys := sortedQuestionKeys(t.Questions)
	questions := make([]*emotionassessment.SurveyQuestion, 0, len(keys))
	for _, qid := range keys {
		m, _ := t.Questions[qid].(map[string]any)
		title := toStr(m["title"])
		if title == "" {
			// 兼容旧结构（键名为 prompt 的种子数据）
			title = toStr(m["prompt"])
		}
		order := int32(trailingNumber(qid))
		if v, ok := m["order"]; ok && toFloat(v) > 0 {
			order = int32(toFloat(v))
		}
		questions = append(questions, &emotionassessment.SurveyQuestion{
			Id:           int64(order),
			Order:        order,
			Prompt:       title, // 历史字段，保持与 title 同值以免旧消费方读到空
			Title:        title,
			Key:          qid,
			QuestionType: toStr(m["type"]),
			OptionItems:  toProtoOptionItems(m["options"]),
			ScaleMin:     int32(toFloat(m["scaleMin"])),
			ScaleMax:     int32(toFloat(m["scaleMax"])),
		})
	}
	return &emotionassessment.Survey{
		Id:          int64(t.ID),
		Code:        t.Code,
		Title:       t.Title,
		Category:    t.Category,
		Version:     int32(t.Version),
		Description: t.Description,
		Questions:   questions,
	}
}

// toProtoSurveyResult 把 types.SubmitSurveyResp 转 proto（E2E-31 #5：分数语义完整）
func toProtoSurveyResult(r *types.SubmitSurveyResp) *emotionassessment.SurveyResult {
	if r == nil {
		return nil
	}
	return &emotionassessment.SurveyResult{
		ResultId:     int64(r.ResultID),
		SurveyId:     int64(r.SurveyID),
		TotalScore:   r.TotalScore,
		Answered:     int32(r.Answered),
		RiskLevel:    r.RiskLevel,
		FactorScores: r.FactorScores,
		ScoreKind:    r.ScoreKind,
	}
}

// toProtoSurveyResultItem 把 types.SurveyResultItem 转 proto（E2E-31 #5）
func toProtoSurveyResultItem(t types.SurveyResultItem) *emotionassessment.SurveyResult {
	return &emotionassessment.SurveyResult{
		ResultId:     int64(t.ResultID),
		SurveyId:     int64(t.SurveyID),
		TotalScore:   t.TotalScore,
		RiskLevel:    t.RiskLevel,
		SubmittedAt:  t.SubmittedAt,
		FactorScores: t.FactorScores,
		ScoreKind:    t.ScoreKind,
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
//
// E2E-31 #3：proto 的 answers 已是 `map<string,int32>`（键 "q1"、值是 option.score），
// 与 logic 层 `types.SubmitSurveyReq.Answers map[string]int` 近乎 1:1，不再需要
// 旧实现的 `Answer oneof` 拆解（那会把 "q1" 数值化、把 option/text 分支降级为占位值）。
func (s *assessmentServer) SubmitSurvey(ctx context.Context, req *emotionassessment.SubmitSurveyRequest) (*emotionassessment.SurveyResult, error) {
	if s.svcCtx == nil || s.svcCtx.SurveyRepo == nil {
		return nil, status.Error(codes.Unavailable, "assessment-svc repository not initialized (degraded start)")
	}
	answers := make(map[string]int, len(req.GetAnswers()))
	for k, v := range req.GetAnswers() {
		answers[k] = int(v)
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

// ListMyResults 实现 ListMyResults RPC
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
//
// E2E-31 #5：旧实现只回 5 个字段（answers / feedback / submittedAt / factorScores /
// scoreKind 全丢），前端"我的结果详情"取不到任何作答明细。
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
		ResultId:     int64(resp.ResultID),
		SurveyId:     int64(resp.SurveyID),
		UserId:       resp.UserID,
		TotalScore:   resp.TotalScore,
		RiskLevel:    resp.RiskLevel,
		DurationSec:  int32(resp.DurationSec),
		Answers:      toProtoAnswers(resp.Answers),
		SubmittedAt:  resp.SubmittedAt,
		FactorScores: resp.FactorScores,
		ScoreKind:    resp.ScoreKind,
	}, nil
}
