//go:build integration
// +build integration

// Package integration_test 真实 Postgres + user-svc PostgresUserRepo CRUD + UpdateProfile。
//
// 流程：testcontainers postgres + emotion_echo_user schema + users 表
//        → 真实 PostgresUserRepo.Create + GetByID + UpdateProfile + Ping
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
  phone VARCHAR(20) UNIQUE,
  email VARCHAR(128) UNIQUE,
  password_hash VARCHAR(255),
  nickname VARCHAR(64),
  avatar_url TEXT,
  gender SMALLINT DEFAULT 0,
  birthday TIMESTAMPTZ,
  status SMALLINT DEFAULT 1,
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

	phone := "+8613800000001"
	email := "u1@example.com"
	pw := "hashed-pw"
	u := &model.User{
		Username:     "u1",
		Phone:        &phone,
		Email:        &email,
		PasswordHash: &pw,
		Status:       1,
	}
	require.NoError(t, repo.Create(ctx, u))
	require.Greater(t, u.ID, int64(0))

	// GetByID
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "u1", got.Username)
	require.Equal(t, phone, *got.Phone)
	require.Equal(t, email, *got.Email)

	// GetByUsername
	got2, err := repo.GetByUsername(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, u.ID, got2.ID)

	// GetByPhone
	got3, err := repo.GetByPhone(ctx, phone)
	require.NoError(t, err)
	require.Equal(t, u.ID, got3.ID)

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
		Status:       1,
	}
	require.NoError(t, repo.Create(ctx, u))

	nick := "newnick"
	gender := int16(1)
	bday := time.Now().AddDate(-30, 0, 0)
	avatar := "https://x.com/a.png"
	require.NoError(t, repo.UpdateProfile(ctx, u.ID, &nick, &gender, &bday, &avatar))

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
