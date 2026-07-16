// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"time"

	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	assessmentServiceName = "emotion-echo-assessment-svc"
	assessmentServiceVer  = "0.1.0"
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
	dbOK := true
	if l.svcCtx.SurveyRepo != nil {
		if err := l.svcCtx.SurveyRepo.Ping(l.ctx); err != nil {
			dbOK = false
		}
	}
	return &types.HealthResp{
		Status:  "ok",
		Time:    time.Now().UnixMilli(),
		Service: assessmentServiceName,
		Version: assessmentServiceVer,
		DbOK:    dbOK,
	}, nil
}