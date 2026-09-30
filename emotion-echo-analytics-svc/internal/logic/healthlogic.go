package logic

import (
	"context"
	"time"

	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"
)

var (
	analyticsServiceName = "emotion-echo-analytics-svc"
	analyticsServiceVer  = "0.1.1"
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
	// ⚠️ 降级启动（E2E-23 F-96 实测修复，2026-09-30）：
	// main.go 的 Postgres 连接是**单次尝试**，失败则 repo 保持 nil 并照常启动。
	// 原写法 `dbOK := true` + `if repo != nil` 跳过整个 if，
	// 于是**恰恰在数据库完全不可用时报 dbOk=true / status=ok** ——
	// 实测该服务 /health/ready 返 200、容器 (healthy)、APISIX 照常路由，
	// 而每个 DB 调用都返 Unavailable，且零告警。
	// 修法：repo 为 nil **本身就是「依赖不可用」**，必须如实报 false。
	dbOK := l.svcCtx.EventRepo != nil
	if dbOK {
		if err := l.svcCtx.EventRepo.Ping(l.ctx); err != nil {
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
		Service: analyticsServiceName,
		Version: analyticsServiceVer,
		DbOK:    dbOK,
	}, nil
}
