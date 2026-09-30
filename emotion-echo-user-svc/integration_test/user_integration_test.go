//go:build integration
// +build integration

// Package integration_test 真实 Postgres + user-svc PostgresUserRepo CRUD + UpdateProfile。
//
// 流程：testcontainers postgres + emotion_echo_user schema + users 表
//
//	→ 真实 PostgresUserRepo.Create + GetByID + UpdateProfile + Ping
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

	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

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

	require.NoError(t, runSQL(ctx, dsn, `CREATE SCHEMA IF NOT EXISTS emotion_echo_user`))
	require.NoError(t, runSQL(ctx, dsn, `
CREATE TABLE IF NOT EXISTS emotion_echo_user.users (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(64) UNIQUE NOT NULL,
  password_hash VARCHAR(255),
  nickname VARCHAR(64),
  avatar_url TEXT,
  gender SMALLINT DEFAULT 0,
  birthday TIMESTAMPTZ,
  config JSONB,                      -- model.User.Config JSONMap；缺这列时
                                       -- Create/UpdateProfile 直接 42703
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
)`))

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

// TestUser_Integration_PostgresCRUD Create + GetByID + 跨用户隔离
func TestUser_Integration_PostgresCRUD(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresUserRepo(db)

	// ⚠️ 本测试此前对着**已废弃的 schema** 写（DDL 里还留着已删除的 phone/email/status 列，
	// 而 model.User 早已没有这三个字段；反过来 model 早已有的 config 列在 DDL 里却缺着）：model.User 早已移除 Phone/Email/Status，
	// repo 也早已移除 GetByPhone/…ByEmail，但集成测试没跟着改 ⇒
	// `go test -tags integration` 直接编译不过，而**默认的 `go test ./...` /
	// `go vet ./...` / CI go-test 全部跳过 build tag 下的文件**，所以坏了很久没人发现
	// （账本 E2E-F-167；anti-patterns AP-09 的"改实现不改测试"跨了 build tag 变体）。
	// 现按当前 schema 重写：断言点从"已删除的联系方式"换成"现存的资料字段 + 现存的方法"。
	nick := "小明"
	gender := int16(1)
	pw := "hashed-pw"
	u := &model.User{
		Username:     "u1",
		Nickname:     &nick,
		Gender:       gender,
		PasswordHash: &pw,
	}
	require.NoError(t, repo.Create(ctx, u))
	require.Greater(t, u.ID, int64(0))

	// GetByID
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "u1", got.Username)
	require.NotNil(t, got.Nickname)
	require.Equal(t, nick, *got.Nickname)
	require.Equal(t, gender, got.Gender)

	// GetByUsername
	got2, err := repo.GetByUsername(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, u.ID, got2.ID)

	// UsernameExists（取代已删除的 GetByPhone 断言位）
	exists, err := repo.UsernameExists(ctx, "u1")
	require.NoError(t, err)
	require.True(t, exists, "刚创建的用户名应存在")
	exists2, err := repo.UsernameExists(ctx, "nope-not-exist")
	require.NoError(t, err)
	require.False(t, exists2, "不存在的用户名应返回 false")

	// Ping
	require.NoError(t, repo.Ping(ctx))
}

// TestUser_Integration_PostgresUpdateProfile UpdateProfile 修改昵称/性别/生日/头像
func TestUser_Integration_PostgresUpdateProfile(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresUserRepo(db)

	pw := "h"
	u := &model.User{
		Username:     "u-edit",
		PasswordHash: &pw,
	}
	require.NoError(t, repo.Create(ctx, u))

	nick := "newnick"
	gender := int16(1)
	bday := time.Now().AddDate(-30, 0, 0)
	avatar := "https://x.com/a.png"
	require.NoError(t, repo.UpdateProfile(ctx, u.ID, &nick, &gender, &bday, &avatar, nil))

	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, &nick, got.Nickname)
	require.Equal(t, gender, got.Gender)
	require.NotNil(t, got.AvatarURL)
	require.Equal(t, avatar, *got.AvatarURL)
}

// TestUser_Integration_PostgresDown 停容器后 Ping 不 panic
func TestUser_Integration_PostgresDown(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	repo := repository.NewPostgresUserRepo(db)

	// 起阶段
	require.NoError(t, repo.Ping(ctx))

	// 停
	require.NoError(t, pgC.Terminate(ctx))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Ping should not panic on db down: %v", r)
		}
	}()
	_ = repo.Ping(ctx) // err 已可接受
}
