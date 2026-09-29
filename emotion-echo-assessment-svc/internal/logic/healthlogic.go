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

// StatusOk / StatusDegraded 构成 D-29 决议的对外契约：status 字段必须说真话。
const (
	StatusOk       = "ok"
	StatusDegraded = "degraded"
)

func (l *HealthLogic) Health() (resp *types.HealthResp, err error) {
	dbOK := true
	if l.svcCtx.SurveyRepo != nil {
		if err := l.svcCtx.SurveyRepo.Ping(l.ctx); err != nil {
			dbOK = false
		}
	}
	// 依赖不可达时如实降级（D-29）。此前 Status 是字面量 "ok" 而 DbOK 变 false，
	// 响应体自相矛盾（E2E-23 测试点 #2/#3）。
	status := StatusOk
	if !dbOK {
		status = StatusDegraded
	}

	return &types.HealthResp{
		Status:  status,
		Time:    time.Now().UnixMilli(),
		Service: assessmentServiceName,
		Version: assessmentServiceVer,
		DbOK:    dbOK,
	}, nil
}
