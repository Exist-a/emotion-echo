// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"time"

	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	chatServiceName = "emotion-echo-chat-svc"
	chatServiceVer  = "0.2.0"
)

// HealthLogic 健康检查
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

// Health 返回健康状态
//
// 报告 DB / Kafka 两个依赖的状态
func (l *HealthLogic) Health() (resp *types.HealthResp, err error) {
	dbOK := true
	if l.svcCtx.ConversationRepo != nil {
		if err := l.svcCtx.ConversationRepo.Ping(l.ctx); err != nil {
			dbOK = false
		}
	}
	kafkaOK := l.svcCtx.EventPublisher != nil

	status := "ok"
	if !dbOK || !kafkaOK {
		status = "degraded"
	}
	return &types.HealthResp{
		Status:  status,
		Time:    time.Now().UnixMilli(),
		Service: chatServiceName,
		Version: chatServiceVer,
		DbOK:    dbOK,
		KafkaOK: kafkaOK,
	}, nil
}