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
	// ⚠️ 降级启动（E2E-23 F-96 实测修复，2026-09-30）：
	// main.go 的 Postgres 连接是**单次尝试**，失败则 repo 保持 nil 并照常启动。
	// 原写法 `dbOK := true` + `if repo != nil` 跳过整个 if，
	// 于是**恰恰在数据库完全不可用时报 dbOk=true / status=ok** ——
	// 实测该服务 /health/ready 返 200、容器 (healthy)、APISIX 照常路由，
	// 而每个 DB 调用都返 Unavailable，且零告警。
	// 修法：repo 为 nil **本身就是「依赖不可用」**，必须如实报 false。
	dbOK := l.svcCtx.ConversationRepo != nil
	if dbOK {
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
