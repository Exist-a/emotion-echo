
package logic

import (
	"context"
	"time"

	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	
)

var (
	chatServiceName = "emotion-echo-chat-svc"
	chatServiceVer  = "0.2.0"
)

// HealthLogic 健康检查
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
// chat-svc 的 status 计算本来就正确（:45-48），此处补常量以便
// /health/ready handler 复用同一判定，避免两边各写一份字面量。
const (
	StatusOk       = "ok"
	StatusDegraded = "degraded"
)

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
	// ⚠️ 已知局限（plan 测试点 #7）：此处只判 EventPublisher 非 nil，
	// 并不真连 Kafka。Kafka 进程挂掉时 kafkaOK 仍为 true —— 见 E2E-23 计划 A2。
	kafkaOK := l.svcCtx.EventPublisher != nil

	status := StatusOk
	if !dbOK || !kafkaOK {
		status = StatusDegraded
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