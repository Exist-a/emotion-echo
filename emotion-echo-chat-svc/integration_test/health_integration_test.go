//go:build integration
// +build integration

// Package integration_test 跑真实 Postgres + chat-svc HealthLogic 端到端集成测试。
//
// 用 testcontainers 起 Postgres 容器，加载 emotion_echo_chat schema，
// 然后用真实 GORM 仓库 + InMemoryEventPublisher 构造 ServiceContext，
// 调 HealthLogic.Health 验证 dbOk=true。
//
// 跑：  go test -tags integration -v ./integration_test/...
package integration_test

import (
	"context"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx stdlib 给 GORM 用
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/logic"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// pgContainerDesc 启动一个 Postgres 容器，加载 emotion_echo_chat schema 与 conversations/messages 表，
// 返回容器句柄和 *gorm.DB。
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

	// 开 schema
	require.NoError(t, runSQL(ctx, dsn,
		`CREATE SCHEMA IF NOT EXISTS emotion_echo_chat`))

	// 建 conversations / messages 表
	require.NoError(t, runSQL(ctx, dsn, `
CREATE TABLE IF NOT EXISTS emotion_echo_chat.conversations (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  title VARCHAR(255),
  message_count INT DEFAULT 0,
  last_message_at TIMESTAMPTZ,
  status SMALLINT DEFAULT 1,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
)`))
	require.NoError(t, runSQL(ctx, dsn, `
CREATE TABLE IF NOT EXISTS emotion_echo_chat.messages (
  id BIGSERIAL PRIMARY KEY,
  conversation_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role VARCHAR(16) NOT NULL,
  content TEXT NOT NULL,
  content_type VARCHAR(16) DEFAULT 'text',
  tokens_used INT DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW()
)`))

	// GORM 打开
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)

	return pgC, db
}

// runSQL 用 GORM 自身执行任意 SQL（无需引入 pgx/pgxpool）
func runSQL(ctx context.Context, dsn string, sql string) error {
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Exec(sql).Error
}

// TestHealthLogic_Integration_RealPostgres 起 Postgres 容器，验证：
//   - HealthLogic 装配 ServiceContext 无错
//   - dbOk=true（连上 Postgres）
//   - repo.Ping 不报错
func TestHealthLogic_Integration_RealPostgres(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() {
		_ = pgC.Terminate(ctx)
	})

	repo := repository.NewPostgresConversationRepo(db)
	pub := events.NewInMemoryEventPublisher()
	svcCtx := svc.NewServiceContext(config.Config{
		Name: "emotion-echo-chat-svc",
	}, repo, pub)

	hl := logic.NewHealthLogic(ctx, svcCtx)
	resp, err := hl.Health()
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Equal(t, "emotion-echo-chat-svc", resp.Service)
	require.Equal(t, "ok", resp.Status)
	require.True(t, resp.DbOK, "dbOk should be true when Postgres is reachable")
	require.Greater(t, resp.Time, int64(0))

	// Ping 应通
	require.NoError(t, repo.Ping(ctx))
}

// TestHealthLogic_Integration_PostgresDown_GracefulDegradation
// 终止容器后 Health 不 panic —— 现行实现可能仍返 ok（取决于 GORM 重试），
// 这里仅断言：不 panic。
func TestHealthLogic_Integration_PostgresDown_GracefulDegradation(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	repo := repository.NewPostgresConversationRepo(db)
	pub := events.NewInMemoryEventPublisher()
	svcCtx := svc.NewServiceContext(config.Config{}, repo, pub)
	hl := logic.NewHealthLogic(ctx, svcCtx)

	// 起阶段：连上 dbOk=true
	resp, err := hl.Health()
	require.NoError(t, err)
	require.True(t, resp.DbOK)

	// 停容器
	require.NoError(t, pgC.Terminate(ctx))

	// 重调不 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Health should not panic on db down, got %v", r)
		}
	}()
	resp2, err := hl.Health()
	// 任何状态都接受（实现可能返 dbOk=false 或 err）
	_ = resp2
	_ = err
}

// TestConversationRepo_Integration_RealPostgresCRUD 真实 Postgres 跑 4 个核心 repo 方法
func TestConversationRepo_Integration_RealPostgresCRUD(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() {
		_ = pgC.Terminate(ctx)
	})

	repo := repository.NewPostgresConversationRepo(db)

	// Create — 使用项目 model 包的实际 Conversation
	c := &model.Conversation{
		UserID: 42,
		Title:  "Integration Test",
		Status: 1,
	}
	require.NoError(t, repo.CreateConversation(ctx, c))
	require.Greater(t, c.ID, int64(0), "DB should auto-increment ID")

	// Read
	got, err := repo.GetConversationByID(ctx, c.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Integration Test", got.Title)
	require.Equal(t, int64(42), got.UserID)

	// IncrementMessageCount
	require.NoError(t, repo.IncrementMessageCount(ctx, c.ID))
	got2, err := repo.GetConversationByID(ctx, c.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got2.MessageCount)

	// AppendMessage + ListMessages
	m := &model.Message{
		ConversationID: c.ID,
		UserID:         42,
		Role:           "user",
		Content:        "hi from integration",
		ContentType:    "text",
	}
	require.NoError(t, repo.AppendMessage(ctx, m))
	require.Greater(t, m.ID, int64(0))

	msgs, err := repo.ListMessages(ctx, c.ID, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "hi from integration", msgs[0].Content)

	// Ping
	require.NoError(t, repo.Ping(ctx))
}
