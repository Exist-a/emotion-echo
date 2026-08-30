//go:build integration
// +build integration

// Stage 30-C A1: ai-svc 消费幂等去重集成测试
//
// 起真 Postgres + apply ai-svc migrations（含 001_add_event_id_to_emotion_analysis.sql）
// + PostgresEmotionRepo.Create 两次同 EventID → 断言表中只有 1 行。
//
// 注意：本次测试需要 ai-svc/migrations/001_*.sql 提供 UNIQUE 约束；migration 文件
// 由本 PR 一并提交。helper 函数 findMigrationsFile 与 analytics-svc 同构（路径
// ../migrations）。
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

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
)

// pgContainerForEmotion 起 postgres + emotion_echo_ai schema + apply ai-svc 全套 migration
func pgContainerForEmotion(t *testing.T, ctx context.Context) (*gorm.DB, func()) {
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
	cleanup := func() { _ = pgC.Terminate(ctx) }

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	require.NoError(t, err)

	require.NoError(t, db.Exec("CREATE SCHEMA IF NOT EXISTS emotion_echo_ai").Error)

	// apply 所有 ai-svc migrations（按文件名排序）
	migDir := findEmotionMigrationsDir(t)
	entries, err := os.ReadDir(migDir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		require.NoError(t, applySQLFile(db, filepath.Join(migDir, e.Name())))
	}

	return db, cleanup
}

// applySQLFile 读 SQL 文件并执行（与 analytics-svc 同包内 helper 一致）
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

// findEmotionMigrationsDir 解析 ai-svc/migrations/ 绝对路径
func findEmotionMigrationsDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	candidates := []string{
		filepath.Join(cwd, "migrations"),
		filepath.Join(cwd, "..", "migrations"),
		filepath.Join(cwd, "..", "..", "migrations"),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	t.Skipf("ai-svc/migrations/ not found from cwd=%s", cwd)
	return ""
}

// TestEmotionRepo_DuplicateEventID_InsertsOnce PG 端 ON CONFLICT 幂等：同 event_id 两次 Create → 1 行
func TestEmotionRepo_DuplicateEventID_InsertsOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEmotionRepo(db)

	first := &model.EmotionAnalysis{
		EventID:        "evt-int-dup-1",
		MessageID:      1001,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.5,
		Model:          "keyword-stub",
	}
	require.NoError(t, repo.Create(ctx, first))
	require.NotZero(t, first.ID, "首次插入应回填 ID")

	// 第二次同 event_id：不应报错（ON CONFLICT DO NOTHING），也不应产生新行
	second := &model.EmotionAnalysis{
		EventID:        "evt-int-dup-1",
		MessageID:      1001,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.5,
		Model:          "keyword-stub",
	}
	require.NoError(t, repo.Create(ctx, second))

	// 断言：表中只有 1 行
	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.emotion_analysis WHERE event_id = ?`, "evt-int-dup-1").
		Scan(&count).Error)
	assert.Equal(t, int64(1), count, "同 event_id 两次 Create 只应落 1 行")

	// 首次插入的 ID 仍可查到
	got, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "evt-int-dup-1", got.EventID)
}

// TestEmotionRepo_DistinctEventIDs_InsertBoth 反例：不同 event_id 各落一行
func TestEmotionRepo_DistinctEventIDs_InsertBoth(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerForEmotion(t, ctx)
	defer cleanup()

	repo := repository.NewPostgresEmotionRepo(db)
	for _, evid := range []string{"evt-int-a", "evt-int-b"} {
		require.NoError(t, repo.Create(ctx, &model.EmotionAnalysis{
			EventID:   evid,
			MessageID: 1,
			Model:     "keyword-stub",
		}))
	}

	var count int64
	require.NoError(t, db.WithContext(ctx).
		Raw(`SELECT COUNT(*) FROM emotion_echo_ai.emotion_analysis`).
		Scan(&count).Error)
	assert.Equal(t, int64(2), count)
}
