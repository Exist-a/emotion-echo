//go:build integration
// +build integration

// testdb_test.go —— ai-svc 集成测试的**共享** Postgres fixture（E2E-F-168，2026-09-30）
//
// ── 为什么要有这个文件 ────────────────────────────────────────────────
// 在此之前，每个测试文件各起各的容器、各写各的建表 DDL。后果（实测）：
// 6 个文件里 3 个调用 `pgContainerForEmotion`（好版本：建 emotion_analysis +
// 跑全部 ai-svc migrations），1 个（grpc_health）自己手写了一套**只建
// emotion_analysis** 的 DDL 且不跑 migration，其余直接复用前者 ——
// 但**前者也不完整**：它只手抄了 `emotion_analysis` 一张表，
// 而 `voice_transcripts` / `face_detections` 是定义在**基础 schema 文件**
// `deploy/db/02-create-tables-in-schemas.sql` 里的、**不在** ai-svc 的
// `migrations/i00*.sql` 里（那些是"在已有表上加列/加约束"的增量迁移）。
// ⇒ 跑完全部 10 个 migration 之后，`voice_transcripts` 依然不存在，
// 14 个测试全红、0 个通过（首个错误 `42P01 relation
// "emotion_echo_ai.voice_transcripts" does not exist`）。
//
// ── 现在的做法（用户 2026-09-30 拍板：共享 fixture 跑真实迁移）─────────
// 测试库结构 = **生产结构**，不再手抄：
//   1. deploy/db/01-create-schemas.sql          （真·建 5 个 schema）
//   2. deploy/db/02-create-tables-in-schemas.sql（真·建全部基础表）
//   3. emotion-echo-ai-svc/migrations/i*.sql     （真·ai-svc 增量迁移，按文件名排序）
// 于是 schema 一演进，测试自动跟上；再也不会出现"测试对着已删除的列写"。
//
// 断言强度的纪律：找不到上面任何一个文件一律 **t.Fatal**，不用 t.Skip ——
// 找不到就跳过 = "没检查到"被当成"检查通过"，正是 E2E-23 反复强调的假绿形态。

package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// repoRootFromTests 从 integration_test/ 出发定位仓库根（cwd = 包目录）。
// 找不到就 Fatal —— 绝不用 Skip 掩盖。
func repoRootFromTests(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err, "取 cwd 失败")
	for _, rel := range []string{
		filepath.Join("..", ".."),
		filepath.Join("..", "..", ".."),
	} {
		p := filepath.Join(cwd, rel)
		if _, err := os.Stat(filepath.Join(p, "deploy", "db", "01-create-schemas.sql")); err == nil {
			return p
		}
	}
	t.Fatalf("定位仓库根失败：cwd=%s 下找不到 deploy/db/01-create-schemas.sql", cwd)
	return ""
}

// newAIDB 起容器并铺好 schema，返回 (*gorm.DB, cleanup)。绝大多数测试用这个。
func newAIDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	db, _, cleanup := newAIDBWithContainer(t)
	return db, cleanup
}

// newAIDBWithContainer 同上，但**额外返回容器句柄**。
// 用途：少数测试要主动关掉容器来验"依赖不可用"路径（如 TestAIGRPC_PostgresDown_*），
// 那类测试不能只拿 cleanup —— 它们要自己控制终止时机。
func newAIDBWithContainer(t *testing.T) (*gorm.DB, *pgcontainer.PostgresContainer, func()) {
	t.Helper()
	ctx := context.Background()

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
	require.NoError(t, err, "起 postgres 容器失败")
	cleanup := func() { _ = pgC.Terminate(ctx) }

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "取容器 DSN 失败")

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	require.NoError(t, err, "连容器失败")

	root := repoRootFromTests(t)

	// 1) 真实基础 schema
	for _, f := range []string{
		"01-create-schemas.sql",
		"02-create-tables-in-schemas.sql",
	} {
		p := filepath.Join(root, "deploy", "db", f)
		require.FileExists(t, p, "找不到基础 schema 文件 %s（测试库必须与生产同源）", p)
		require.NoError(t, applySQLFile(db, p), "应用 %s 失败", f)
	}

	// 2) ai-svc 增量迁移（按文件名排序，与 deploy/db/migrate.sh 一致）
	migDir := filepath.Join(root, "emotion-echo-ai-svc", "migrations")
	entries, err := os.ReadDir(migDir)
	require.NoError(t, err, "读 ai-svc 迁移目录失败：%s", migDir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		names = append(names, e.Name())
	}
	require.NotEmpty(t, names, "ai-svc/migrations 下没有 .sql —— 迁移目录异常")
	sort.Strings(names)
	for _, n := range names {
		require.NoError(t, applySQLFile(db, filepath.Join(migDir, n)), "应用迁移 %s 失败", n)
	}

	return db, pgC, cleanup
}

// applySQLFile 读 SQL 文件并整段执行。
// ⚠️ 用 Exec 跑**整个文件**（不是逐语句）：GORM/PG 的 simple protocol 允许
// 多语句；迁移文件里有 `DO $$ ... $$` 这类含分号的块，逐语句切分会切坏。
func applySQLFile(db *gorm.DB, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return db.Exec(string(b)).Error
}
