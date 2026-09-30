package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 A 组 · 测试点 #2：`/health` 的 status 字段必须说真话。
//
// 🔴 背景（计划期实测，plan §0 F-a）：`healthlogic.go` 把 Status 写死为字面量
// "ok"，而同一响应里的 DbOK 会如实变 false ⇒ 依赖挂了、响应体却自报健康。
// 本测试锁定 D-29 决议的契约：**依赖不通时 status 必须报 degraded**。
//
// 既有 healthlogic_test.go 只覆盖 repo 为 nil 的 happy path，从无任何断言
// 触碰 Status 字段在"依赖失败"时的取值 —— 这正是该缺陷能长期存活的原因。

// pingUserRepo 是只关心 Ping 的测试替身：其余方法 panic，任何误用都会立刻暴露。
type pingUserRepo struct {
	pingErr error
}

func (f *pingUserRepo) Ping(ctx context.Context) error { return f.pingErr }

func (f *pingUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	panic("健康检查测试不应调用 GetByID")
}
func (f *pingUserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	panic("健康检查测试不应调用 GetByUsername")
}
func (f *pingUserRepo) Create(ctx context.Context, u *model.User) error {
	panic("健康检查测试不应调用 Create")
}
func (f *pingUserRepo) UpdateProfile(ctx context.Context, id int64, nickname *string, gender *int16, birthday *time.Time, avatarURL *string, config *model.JSONMap) error {
	panic("健康检查测试不应调用 UpdateProfile")
}
func (f *pingUserRepo) UsernameExists(ctx context.Context, username string) (bool, error) {
	panic("健康检查测试不应调用 UsernameExists")
}
func (f *pingUserRepo) UpdatePassword(ctx context.Context, id int64, newPasswordHash string) error {
	panic("健康检查测试不应调用 UpdatePassword")
}

var _ repository.UserRepo = (*pingUserRepo)(nil)

func newHealthLogicWithRepo(repo repository.UserRepo) *HealthLogic {
	return NewHealthLogic(context.Background(), &svc.ServiceContext{
		Config:   config.Config{},
		UserRepo: repo,
	})
}

// 测试点 #2：DB 通 ⇒ ok；DB 不通 ⇒ degraded（不是 ok）。
func TestHealthLogic_Health_StatusReflectsDatabaseState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus string
		wantDbOK   bool
	}{
		{
			name:       "db reachable returns ok",
			pingErr:    nil,
			wantStatus: "ok",
			wantDbOK:   true,
		},
		{
			// 🔴 本条在修复前必然失败：实现写死 Status="ok"（healthlogic.go:41）
			name:       "db unreachable returns degraded not ok",
			pingErr:    errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"),
			wantStatus: "degraded",
			wantDbOK:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := newHealthLogicWithRepo(&pingUserRepo{pingErr: tt.pingErr})
			resp, err := l.Health()

			require.NoError(t, err, "依赖不可达不应是 handler 级错误，应由 status 字段表达")
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantStatus, resp.Status,
				"status 必须反映依赖真实状态（依赖挂了还报 ok = 探针说假话）")
			assert.Equal(t, tt.wantDbOK, resp.DbOK, "DbOK 必须如实反映 ping 结果")
			assert.NotEmpty(t, resp.Service, "service 字段必须有值（排障时需要定位是哪个服务）")
			assert.NotEmpty(t, resp.Version, "version 字段必须有值")
		})
	}
}

// 测试点 #2 的反面对照：若实现退化为"永远 ok"，本测试必须红。
// 显式断言"status 与 dbOk 不得同时声称健康"，杜绝二者再次脱钩。
func TestHealthLogic_Health_StatusAndDbOKNeverDisagree(t *testing.T) {
	t.Parallel()

	l := newHealthLogicWithRepo(&pingUserRepo{pingErr: errors.New("boom")})
	resp, err := l.Health()
	require.NoError(t, err)

	assert.False(t, resp.DbOK, "DbOK 应为 false")
	assert.NotEqual(t, "ok", resp.Status,
		"DbOK=false 时 status 绝不能是 ok —— 响应体不得自相矛盾")
}

// 🔴 契约反转（2026-09-30，E2E-23 F-96 实测驱动）：repo 为 nil **必须**报 degraded。
//
// 本测试原先断言相反，注释写的是：
//   「repo 为 nil 时保持 ok：未接入依赖不等于依赖故障（dev 单测场景大量依赖此行为）」
//
// **那句推理把两件不同的事当成了同一件**：状态变量 `repo == nil` 同时表示
//  ①「这个部署本来就不接 DB」（本项目 5 个服务**都不是**这种情况）
//  ②「main.go 的单次连接失败了，降级启动」（**生产中真实发生**，`main.go:85-93`）
// 按 ① 论证而让 ② 也报 ok，代价是（dev 栈实测，降级态下 user-svc）：
//   · `/health`        → `{"status":"ok","dbOk":true}`
//   · `/health/ready`  → **200**
//   · `docker ps`      → 容器 **(healthy)**
//   · 经网关 `/api/v1/users/me` → `upstream unavailable:
//     user-svc repository not initialized (degraded start)`
// ⇒ compose 的 `service_healthy` 闸门（含 apisix-seed）失效、APISIX 照常路由、
//   **后端全挂而零告警**。
//
// 注释里说的「不谎报失败」恰恰说反了：它不是避免谎报，而是**在谎报健康**。
// 「dev 单测大量依赖此行为」是真问题，但解法是**让那些测试注入 repo**
// （见 healthlogic_test.go 的 newTestHealthLogic），而不是让探针对
// 缺依赖说谎。
func TestHealthLogic_Health_NilRepoReportsDegraded(t *testing.T) {
	t.Parallel()

	l := newHealthLogicWithRepo(nil)
	resp, err := l.Health()

	require.NoError(t, err)
	assert.Equal(t, StatusDegraded, resp.Status,
		"repo 为 nil（生产降级启动的真实形态）时 status 必须是 degraded —— "+
			"报 ok 会让 compose service_healthy 闸门与 APISIX 路由双双失明")
	assert.False(t, resp.DbOK,
		"repo 为 nil 时 DbOK 必须是 false：数据库层根本不存在，不存在'依赖正常'这回事")
}
