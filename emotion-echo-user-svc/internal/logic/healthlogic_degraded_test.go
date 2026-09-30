package logic

import (
	"errors"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 · 「降级启动时健康探针必须说假话」
//
// 🔴 缺陷来源（2026-09-30 F-96 实测抓出，非推理）：
//
//	user-svc/main.go:85-93 的降级启动是**单次连接尝试**——失败则
//	`userRepo` 保持 nil 并照常启动。而原 `healthlogic.Health()` 写成：
//
//	    dbOK := true
//	    if l.svcCtx.UserRepo != nil {          // ← repo 为 nil 时整个 if 被跳过
//	        if err := l.svcCtx.UserRepo.Ping(ctx); err != nil { dbOK = false }
//	    }
//
//	⇒ **恰恰在数据库完全不可用时** dbOK 保持 true、status 保持 ok。
//
//	实测（dev 栈，停 Postgres + 强制重建 user-svc）：
//	  · 日志出现 `connect failed`（降级启动已发生）
//	  · `/health`       → `{"status":"ok","dbOk":true}`
//	  · `/health/ready` → **200**
//	  · `docker ps`     → 容器 **(healthy)**
//	  · 经网关 `/api/v1/users/me` → `upstream unavailable:
//	    user-svc repository not initialized (degraded start)`
//
//	危害：compose healthcheck 打的正是 `/health/ready` ⇒ 降级服务被判 healthy
//	⇒ 任何 `depends_on: condition: service_healthy` 闸门（含 apisix-seed）失效
//	⇒ APISIX 照常路由，而该服务每个 DB 调用都返 Unavailable，且**零告警**。
//
//	既有 `healthlogic_contract_test.go`（D-29 契约）只覆盖了
//	"repo 存在但 Ping 失败"，**从未覆盖"repo 根本没建起来"** ——
//	这正是该缺陷能带着 PASS 存活的原因。
var errPingForTest = errors.New("ping failed for test")

func TestHealth_NilRepo_ReportsDegradedNotOK(t *testing.T) {
	// 直接构造 repo 为 nil 的 ServiceContext，即 main.go 降级启动后的真实形态
	l := NewHealthLogic(t.Context(), &svc.ServiceContext{
		Config:   config.Config{},
		UserRepo: nil, // ← 降级启动：单次连接失败后 repo 保持 nil
	})

	resp, err := l.Health()
	require.NoError(t, err, "健康检查本身不应返回 error（它要说真话，而不是失败）")

	assert.False(t, resp.DbOK,
		"repo 为 nil（降级启动）时 DbOK 必须是 false。"+
			"原实现 `dbOK := true` + `if repo != nil` 在 repo 为 nil 时跳过整个 if，"+
			"导致**恰恰在数据库完全不可用时报健康**")
	assert.Equal(t, StatusDegraded, resp.Status,
		"repo 为 nil 时 status 必须是 degraded —— D-29 契约：status 字段必须说真话")
}

// 同一缺陷的第二个面：repo 存在但 Ping 失败（既有契约已覆盖，此处并列以防回退）
func TestHealth_RepoPingFails_ReportsDegradedNotOK(t *testing.T) {
	l := newHealthLogicWithRepo(&pingUserRepo{pingErr: errPingForTest})

	resp, err := l.Health()
	require.NoError(t, err)
	assert.False(t, resp.DbOK, "Ping 失败时 DbOK 必须为 false")
	assert.Equal(t, StatusDegraded, resp.Status, "Ping 失败时 status 必须是 degraded")
}

// 对照组：repo 存在且 Ping 成功 ⇒ 必须报 ok。
//
// 这一条同样重要：只测"失败"不测"成功"的守卫，会被"永远返回 degraded"的
// 实现蒙混过关（本项目此前已吃过一次这种亏 —— 弱断言）。
func TestHealth_RepoPingOK_ReportsOK(t *testing.T) {
	l := newHealthLogicWithRepo(&pingUserRepo{pingErr: nil})

	resp, err := l.Health()
	require.NoError(t, err)
	assert.True(t, resp.DbOK, "Ping 成功时 DbOK 必须为 true")
	assert.Equal(t, StatusOk, resp.Status, "Ping 成功时 status 必须是 ok")
}
