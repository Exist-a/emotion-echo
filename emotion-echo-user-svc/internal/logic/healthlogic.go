// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"time"

	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	serviceName = "emotion-echo-user-svc"
	serviceVer  = "0.1.1"
)

type HealthLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHealthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HealthLogic {
	return &HealthLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *HealthLogic) Health() (resp *types.HealthResp, err error) {
	// ping DB
	dbOK := true
	if l.svcCtx.UserRepo != nil {
		if err := l.svcCtx.UserRepo.Ping(l.ctx); err != nil {
			dbOK = false
		}
	}
	return &types.HealthResp{
		Status:  "ok",
		Time:    time.Now().UnixMilli(),
		Service: serviceName,
		Version: serviceVer,
		DbOK:    dbOK,
	}, nil
}