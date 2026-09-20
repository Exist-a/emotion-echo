package logic

import (
	"context"
	"errors"

	"emotion-echo-assessment-svc/internal/middleware"
	"emotion-echo-assessment-svc/internal/model"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/scoring"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"
)

type GetSurveyResultLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSurveyResultLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSurveyResultLogic {
	return &GetSurveyResultLogic{

		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetSurveyResult 查询单条量表结果（带鉴权：必须是自己的）
func (l *GetSurveyResultLogic) GetSurveyResult(req *types.GetSurveyResultReq) (resp *types.GetSurveyResultResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id in context")
	}

	if req.ResultId == 0 {
		return nil, errors.New("validation: result id is required")
	}

	res, err := l.svcCtx.SurveyRepo.GetResult(l.ctx, req.ResultId, uid)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, repository.ErrNotFound
	}

	return &types.GetSurveyResultResp{
		ResultID:     res.ID,
		SurveyID:     res.SurveyID,
		UserID:       res.UserID,
		TotalScore:   res.TotalScore,
		RiskLevel:    res.RiskLevel,
		DurationSec:  res.DurationSec,
		Answers:      res.Answers,
		SubmittedAt:  res.SubmittedAt.UnixMilli(),
		FactorScores: jsonMapToFloat64(res.FactorScores),
		ScoreKind:    scoreKindFromRiskLevel(res.RiskLevel),
	}, nil
}

// scoreKindFromRiskLevel 从落库的 riskLevel 反推 totalScore 的语义（E2E-F-97 ①）。
//
// 读结果时 scorer 已不再参与（分数是存下来的），故用同一个判别标记还原语义：
// 人格量表写死 "dimension_profile" ⇒ 其 totalScore 是五维度之和，无严重度语义。
func scoreKindFromRiskLevel(riskLevel string) string {
	if riskLevel == "dimension_profile" {
		return scoring.KindDimensionSum
	}
	return scoring.KindRisk
}

// jsonMapToFloat64 model.JSONMap → map[string]float64
// （JSONB 反序列化后数值是 float64；非数值项跳过而非报错，避免脏数据让整条结果读不出）
func jsonMapToFloat64(m model.JSONMap) map[string]float64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ListMyResults 列出当前用户所有量表结果
func (l *GetSurveyResultLogic) ListMyResults(req *types.ListMyResultsReq) (resp *types.ListMyResultsResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id in context")
	}

	limit := 20
	if req != nil && req.Limit > 0 && req.Limit <= 100 {
		limit = req.Limit
	}

	results, err := l.svcCtx.SurveyRepo.ListResultsByUser(l.ctx, uid, limit)
	if err != nil {
		return nil, err
	}

	resp = &types.ListMyResultsResp{
		Items: make([]types.SurveyResultItem, 0, len(results)),
		Total: len(results),
	}
	for _, r := range results {
		resp.Items = append(resp.Items, types.SurveyResultItem{
			ResultID:     r.ID,
			SurveyID:     r.SurveyID,
			TotalScore:   r.TotalScore,
			RiskLevel:    r.RiskLevel,
			SubmittedAt:  r.SubmittedAt.UnixMilli(),
			FactorScores: jsonMapToFloat64(r.FactorScores),
			ScoreKind:    scoreKindFromRiskLevel(r.RiskLevel),
		})
	}
	return resp, nil
}
