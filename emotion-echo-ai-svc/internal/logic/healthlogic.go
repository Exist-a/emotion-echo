// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"time"

	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	aiServiceName = "emotion-echo-ai-svc"
	aiServiceVer  = "0.1.1"
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
	if l.svcCtx.EmotionRepo != nil {
		if err := l.svcCtx.EmotionRepo.Ping(l.ctx); err != nil {
			dbOK = false
		}
	}
	return &types.HealthResp{
		Status:  "ok",
		Time:    time.Now().UnixMilli(),
		Service: aiServiceName,
		Version: aiServiceVer,
		DbOK:    dbOK,
	}, nil
}