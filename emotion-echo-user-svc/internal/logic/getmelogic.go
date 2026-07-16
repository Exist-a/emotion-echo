// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"errors"

	"emotion-echo-user-svc/internal/middleware"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMeLogic {
	return &GetMeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetMe 取当前登录用户
//
// 鉴权流程（mock）：
//  1. middleware.AuthMiddleware 从 X-User-Id header 拿到 uid
//  2. 把 uid 塞到 ctx（key = middleware.CtxUserIDKey）
//  3. logic 从 ctx 拿 uid
//  4. 调 repo 查 user
func (l *GetMeLogic) GetMe(req *types.GetMeReq) (resp *types.GetMeResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid == 0 {
		return nil, errors.New("unauthorized: missing user id in context")
	}

	u, err := l.svcCtx.UserRepo.GetByID(l.ctx, uid)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, repository.ErrNotFound
	}

	return toGetMeResp(u), nil
}

func toGetMeResp(u *model.User) *types.GetMeResp {
	phone := ""
	if u.Phone != nil {
		phone = *u.Phone
	}
	nick := ""
	if u.Nickname != nil {
		nick = *u.Nickname
	}
	return &types.GetMeResp{
		User: types.UserInfo{
			UserId:   u.ID,
			Account:  u.Username,
			Phone:    phone,
			Nickname: nick,
		},
	}
}