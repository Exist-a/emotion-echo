// Code scaffolded by goctl. Safe to edit.

package logic

import (
	"context"
	"errors"

	"emotion-echo-assessment-svc/internal/middleware"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetSurveyResultLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSurveyResultLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSurveyResultLogic {
	return &GetSurveyResultLogic{
		Logger: logx.WithContext(ctx),
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
		ResultID:    res.ID,
		SurveyID:    res.SurveyID,
		UserID:      res.UserID,
		TotalScore:  res.TotalScore,
		RiskLevel:   res.RiskLevel,
		DurationSec: res.DurationSec,
		Answers:     res.Answers,
		SubmittedAt: res.SubmittedAt.UnixMilli(),
	}, nil
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
			ResultID:    r.ID,
			SurveyID:    r.SurveyID,
			TotalScore:  r.TotalScore,
			RiskLevel:   r.RiskLevel,
			SubmittedAt: r.SubmittedAt.UnixMilli(),
		})
	}
	return resp, nil
}