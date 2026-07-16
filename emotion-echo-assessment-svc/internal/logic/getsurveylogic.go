// Code scaffolded by goctl. Safe to edit.

package logic

import (
	"context"
	"errors"

	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetSurveyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSurveyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSurveyLogic {
	return &GetSurveyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetSurvey 返回量表详情（包含题目内容）
func (l *GetSurveyLogic) GetSurvey(req *types.GetSurveyReq) (resp *types.GetSurveyResp, err error) {
	if req.Id == 0 {
		return nil, errors.New("validation: survey id is required")
	}

	s, err := l.svcCtx.SurveyRepo.GetByID(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, repository.ErrNotFound
	}

	return &types.GetSurveyResp{
		ID:        s.ID,
		Code:      s.Code,
		Title:     s.Title,
		Category:  s.Category,
		Version:   s.Version,
		Questions: s.Questions,
	}, nil
}