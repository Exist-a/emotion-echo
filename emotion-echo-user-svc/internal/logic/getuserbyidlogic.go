
package logic

import (
	"context"
	"errors"

	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	
)

type GetUserByIdLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserByIdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserByIdLogic {
	return &GetUserByIdLogic{

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
	// E2E-11：统一走 toUserInfo（原先此处是独立内联映射，只填 3 个字段，
	// 导致 GetMe 修好 avatar/createdAt 后本路径仍丢 —— 同一实体两处映射必然漂移）。
	return &types.GetUserByIdResp{User: toUserInfo(u)}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}