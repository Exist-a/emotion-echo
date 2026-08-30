//go:build integration
// +build integration

// Package integration_test 跑真实 Postgres + analytics-svc Kafka consumer
// 写入路径的端到端集成测试（Stage 30-A §三.3 + §七）。
//
// 流程：
//   1. testcontainers 起 postgres:15-alpine
//   2. 建 emotion_echo_analytics schema + apply migration 002 (user_behavior_events)
//   3. 用真 PostgresEventRepo.Create 写入一行
//   4. 用真 PostgresEventRepo.GetByID 读出，断言字段一致
//
// 跑：go test -tags integration -v ./integration_test/...
//
// 注意：Round 5 GREEN part 3 仅覆盖 EventRepo 真 SQL 路径；
// ReportRepo / MentalHealthRepo 的真 SQL 是独立 PR（需要
// 跨 schema VIEW 已建好才能跑；本次已经一并 apply migrations 001-004）。
package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"
)

// pgContainerForEvents 起 postgres + 建 analytics schema + apply migration 002
func pgContainerForEvents(t *testing.T, ctx context.Context) (*gorm.DB, func()) {
	t.Helper()

	pg, err := pgcontainer.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		pgcontainer.WithDatabase("emotion_echo_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	cleanup := func() {
		_ = pg.Terminate(ctx)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	require.NoError(t, err)

	// 建 emotion_echo_analytics schema
	require.NoError(t, db.Exec("CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics").Error)

	// apply migration 002 (基础表) + 006 (Stage 30-C A1 event_id 列 + UNIQUE)
	mig002 := findMigrationsFile(t, "002_create_user_behavior_events.sql")
	require.NoError(t, applySQLFile(db, mig002))
	mig006 := findMigrationsFile(t, "006_add_event_id_to_user_behavior_events.sql")
	require.NoError(t, applySQLFile(db, mig006))

	return db, cleanup
}

func findMigrationsFile(t *testing.T, name string) string {
	t.Helper()
	candidates := []string{
		filepath.Join("migrations", name),
		filepath.Join("..", "migrations", name),
		filepath.Join("..", "..", "migrations", name),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skipf("migrations/%s not found", name)
	return ""
}

func applySQLFile(db *gorm.DB, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(string(b))
	return err
}

// =====================================================
// TestUserBehaviorEvent_Create_And_GetById_Integration
// =====================================================

func TestUserBehaviorEvent_Create_And_GetById_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEvents(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEventRepo(db)

	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	ev := &model.UserBehaviorEvent{
		UserID:     42,
		EventType:  "message",
		Target:     "evt-int-1",
		SessionID:  "chat-events",
		OccurredAt: now,
	}
	require.NoError(t, repo.Create(ctx, ev))

	// auto-increment 应已填充 ID
	assert.NotZero(t, ev.ID, "Create 应回填 ID（autoincrement）")

	got, err := repo.GetByID(ctx, ev.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(42), got.UserID)
	assert.Equal(t, "message", got.EventType)
	assert.Equal(t, "evt-int-1", got.Target)
	assert.Equal(t, "chat-events", got.SessionID)
	assert.True(t, got.OccurredAt.Equal(now), "OccurredAt 应回填 %v, got %v", now, got.OccurredAt)
}

func TestUserBehaviorEvent_GetByID_NotFound_ReturnsNilNil(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEvents(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEventRepo(db)

	// 不存在的 id
	got, err := repo.GetByID(ctx, 99999)
	require.NoError(t, err)
	assert.Nil(t, got, "不存在的 id 返 (nil, nil)，调用方决定 404 vs empty")
}

func TestUserBehaviorEvent_Ping_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEvents(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEventRepo(db)
	require.NoError(t, repo.Ping(ctx))
}