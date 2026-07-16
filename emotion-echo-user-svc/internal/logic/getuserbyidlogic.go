// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"errors"

	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserByIdLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserByIdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserByIdLogic {
	return &GetUserByIdLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserByIdLogic) GetUserById(req *types.GetUserByIdReq) (resp *types.GetUserByIdResp, err error) {
	if req.Id <= 0 {
		return nil, errors.New("invalid user id")
	}
	u, err := l.svcCtx.UserRepo.GetByID(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, repository.ErrNotFound
	}
	return &types.GetUserByIdResp{User: types.UserInfo{
		UserId:   u.ID,
		Account:  u.Username,
		Phone:    derefString(u.Phone),
		Nickname: derefString(u.Nickname),
	}}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}