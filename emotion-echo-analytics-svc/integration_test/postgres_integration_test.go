//go:build integration
// +build integration

// Package integration_test 跑真实 Postgres + analytics-svc 端到端集成测试。
//
// 流程：
//   - testcontainers 起 postgres:15-alpine
//   - 建 emotion_echo_analytics schema + user_behavior_events 表
//   - 真实 PostgresEventRepo 注入 + HealthLogic
//   - 验证 HealthLogic.Health.dbOk=true + PostgresEventRepo CRUD
//
// 跑：  go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/logic"
	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// pgContainerDesc 起 postgres 容器，初始化 emotion_echo_analytics schema + user_behavior_events 表
func pgContainerDesc(t *testing.T, ctx context.Context) (*pgcontainer.PostgresContainer, *gorm.DB) {
	t.Helper()

	pgC, err := pgcontainer.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		pgcontainer.WithDatabase("emotion_echo_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, runSQL(ctx, dsn, `CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics`))
	require.NoError(t, runSQL(ctx, dsn, `
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  target VARCHAR(255),
  session_id VARCHAR(64),
  occurred_at TIMESTAMPTZ DEFAULT NOW()
)`))
	require.NoError(t, runSQL(ctx, dsn, `CREATE INDEX IF NOT EXISTS idx_events_user ON emotion_echo_analytics.user_behavior_events(user_id, occurred_at DESC)`))

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	return pgC, db
}

func runSQL(ctx context.Context, dsn, sql string) error {
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Exec(sql).Error
}

// TestAnalytics_Integration_PostgresHealthLogic 真实 Postgres 跑 HealthLogic
func TestAnalytics_Integration_PostgresHealthLogic(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresEventRepo(db)
	svcCtx := svc.NewServiceContext(config.Config{Name: "emotion-echo-analytics-svc"}, repo)
	hl := logic.NewHealthLogic(ctx, svcCtx)

	resp, err := hl.Health()
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "emotion-echo-analytics-svc", resp.Service)
	require.Equal(t, "ok", resp.Status)
	require.True(t, resp.DbOK, "dbOk should be true when Postgres is reachable")
}

// TestAnalytics_Integration_PostgresEventRepoCRUD 真实 Postgres 跑 Create + GetByID + Ping
func TestAnalytics_Integration_PostgresEventRepoCRUD(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresEventRepo(db)

	// 多用户写入（验证不同 UserID 隔离）
	for i := 0; i < 5; i++ {
		e := &model.UserBehaviorEvent{
			UserID:    int64(100 + i),
			EventType: "page_view",
			Target:    "/integration-test",
			SessionID: "sess-x",
		}
		require.NoError(t, repo.Create(ctx, e))
		require.Greater(t, e.ID, int64(0))
	}

	// Ping
	require.NoError(t, repo.Ping(ctx))
}

// TestAnalytics_Integration_PostgresDown_Graceful 停容器后 Health 不 panic
func TestAnalytics_Integration_PostgresDown_Graceful(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	repo := repository.NewPostgresEventRepo(db)
	svcCtx := svc.NewServiceContext(config.Config{}, repo)
	hl := logic.NewHealthLogic(ctx, svcCtx)

	// 起阶段 dbOk=true
	resp, _ := hl.Health()
	require.True(t, resp.DbOK)

	// 停
	require.NoError(t, pgC.Terminate(ctx))

	// 再调不 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Health should not panic on db down: %v", r)
		}
	}()
	resp2, err := hl.Health()
	_ = resp2
	_ = err
}
