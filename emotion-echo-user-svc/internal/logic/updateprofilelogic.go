// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"errors"
	"time"

	"emotion-echo-user-svc/internal/middleware"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateProfileLogic 处理 PATCH /api/v1/users/me
//
// 流程：
//  1. 从 ctx 拿到当前登录用户 id（由 GinAuthMiddleware 注入）
//  2. 校验入参（昵称长度、性别枚举、生日格式）
//  3. 调 UserRepo.UpdateProfile 局部更新
//  4. 返回最新的用户信息
type UpdateProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProfileLogic {
	return &UpdateProfileLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

const maxNicknameLen = 32

// UpdateProfile 修改当前用户可编辑字段
func (l *UpdateProfileLogic) UpdateProfile(req *types.UpdateProfileReq) (resp *types.UpdateProfileResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id in context")
	}

	// 校验
	if req.Nickname != nil {
		if len(*req.Nickname) == 0 || len(*req.Nickname) > maxNicknameLen {
			return nil, errors.New("validation: nickname length must be 1-32")
		}
	}
	if req.Gender != nil {
		if *req.Gender < 0 || *req.Gender > 2 {
			return nil, errors.New("validation: gender must be 0(unknown)/1(male)/2(female)")
		}
	}

	var birthday *time.Time
	if req.Birthday != nil && *req.Birthday != "" {
		t, perr := time.Parse("2006-01-02", *req.Birthday)
		if perr != nil {
			return nil, errors.New("validation: birthday must be YYYY-MM-DD")
		}
		birthday = &t
	}

	if err := l.svcCtx.UserRepo.UpdateProfile(l.ctx, uid, req.Nickname, req.Gender, birthday, req.AvatarURL); err != nil {
		return nil, err
	}

	// 重新查一次返回最新值
	u, err := l.svcCtx.UserRepo.GetByID(l.ctx, uid)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, repository.ErrNotFound
	}
	return toGetMeResp(u), nil // 复用 getmelogic 的转换函数，结构完全一致
}