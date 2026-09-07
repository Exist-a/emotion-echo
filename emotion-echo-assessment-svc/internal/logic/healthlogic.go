
package logic

import (
	"context"
	"time"

	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	
)

var (
	assessmentServiceName = "emotion-echo-assessment-svc"
	assessmentServiceVer  = "0.1.0"
)

type HealthLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHealthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HealthLogic {
	return &HealthLogic{

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