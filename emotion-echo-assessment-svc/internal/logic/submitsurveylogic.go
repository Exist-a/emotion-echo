// Code scaffolded by goctl. Safe to edit.

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

	"github.com/zeromicro/go-zero/core/logx"
)

type SubmitSurveyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSubmitSurveyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitSurveyLogic {
	return &SubmitSurveyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SubmitSurvey 处理作答提交
//
// 计分：使用 scoring.Score() 自动按 survey.Code 选合适的 scorer
//   - PHQ-9：9 题 0-3，5 档分级（none/mild/moderate/severe/extreme）
//   - GAD-7：7 题 0-3，4 档分级（none/mild/moderate/severe）
//   - PSQI ：7 个 component 0-3，4 档分级（none/mild/moderate/severe）
//   - 其他：GenericScorer（兜底）
//
// 之前 Phase 4 简化版本（sum + 0.4/0.7 阈值）已被 scoring 包取代
func (l *SubmitSurveyLogic) SubmitSurvey(req *types.SubmitSurveyReq) (resp *types.SubmitSurveyResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id in context")
	}

	if req.SurveyId == 0 {
		return nil, errors.New("validation: survey id is required")
	}

	// 校验量表存在
	s, err := l.svcCtx.SurveyRepo.GetByID(l.ctx, req.SurveyId)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, repository.ErrNotFound
	}

	// 校验 answers 非空
	if len(req.Answers) == 0 {
		return nil, errors.New("validation: answers cannot be empty")
	}

	// 计分（按 survey code 自动选规则）
	scoreResult, err := scoring.Score(s, req.Answers)
	if err != nil {
		return nil, err
	}

	result := &model.SurveyResult{
		UserID:      uid,
		SurveyID:    req.SurveyId,
		Answers:     toJSONMap(req.Answers),
		FactorScores: toJSONMapFloat64(scoreResult.Factors),
		TotalScore:  scoreResult.TotalScore,
		RiskLevel:   scoreResult.RiskLevel,
		DurationSec: req.DurationSec,
	}

	if err := l.svcCtx.SurveyRepo.SaveResult(l.ctx, result); err != nil {
		return nil, err
	}

	// answered = 实际回答数
	answered := len(req.Answers)

	return &types.SubmitSurveyResp{
		ResultID:   result.ID,
		SurveyID:   result.SurveyID,
		TotalScore: result.TotalScore,
		Answered:   answered,
		RiskLevel:  result.RiskLevel,
	}, nil
}

// toJSONMap map[string]int → model.JSONMap
func toJSONMap(m map[string]int) model.JSONMap {
	out := make(model.JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// toJSONMapFloat64 map[string]float64 → model.JSONMap（用于 factor_scores）
func toJSONMapFloat64(m map[string]float64) model.JSONMap {
	out := make(model.JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}