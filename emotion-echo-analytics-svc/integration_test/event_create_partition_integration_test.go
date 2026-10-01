//go:build integration
// +build integration

// event_create_partition_integration_test.go —— E2E-F-149 回归钉
//
// ── 背景 ─────────────────────────────────────────────────────────────
// a008 把 user_behavior_events 改为按 occurred_at 月分区后，分区表上的
// UNIQUE 约束必须包含分区键 ⇒ uq_user_behavior_events_partitioned_event_id
// 是 UNIQUE (event_id, occurred_at)。而 PostgresEventRepo.Create 的
// ON CONFLICT 冲突目标只写了 (event_id) ⇒ Postgres 报
//   42P10 there is no unique or exclusion constraint matching the
//        ON CONFLICT specification
// ⇒ consumer 每条消息写库必失败（账本 F-149 2026-09-29 实测 + 2026-10-01
// E2E-24 开工复核复现：发消息 → attempt 1/3 → 4 → DLQ，表行数不变）。
//
// ── 与既有 fixture 的差别 ────────────────────────────────────────────
// events_integration_test.go 的 pgContainerForEvents 只铺 002+006——
// **不含 a008** ⇒ 那套 fixture 上的 Create 永远跑在非分区表上，看不见本缺陷。
// 本文件自建 fixture：002 → 006 → a008（真迁移链到分区表）。
// a008 含 psql 元命令（\set），database/sql 不认 ⇒ 剥掉以 `\\` 开头的行再执行。
//
// 跑：go test -tags integration -run Partitioned ./integration_test/ -v
package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// pgContainerPartitionedEvents 起 postgres + 002/006/a008 真迁移链 ⇒ 分区表 schema。
// 找不到任何迁移文件一律 Fatal（E2E-F-168 纪律：不许用 Skip 掩盖）。
func pgContainerPartitionedEvents(t *testing.T, ctx context.Context, extraMigrations ...string) (*gorm.DB, func()) {
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
	cleanup := func() { _ = pg.Terminate(ctx) }

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.Exec("CREATE SCHEMA IF NOT EXISTS emotion_echo_analytics").Error)

	root := repoRootFromIntegration(t)
	for _, rel := range []string{
		filepath.Join("deploy", "db", "01-create-schemas.sql"),
		filepath.Join("deploy", "db", "02-create-tables-in-schemas.sql"),
	} {
		p := filepath.Join(root, rel)
		require.FileExists(t, p, "找不到基础 schema 文件 %s（测试库必须与生产同源）", p)
		require.NoError(t, applySQLFile(db, p), "apply %s", rel)
	}

	// a006：event_id 列 + UNIQUE（分区化前置）
	applyMigration(t, db, root, "a006_add_event_id_to_user_behavior_events.sql")

	// 钩子：在 a008（复制老数据 + 换表）之前造老数据
	if seedLegacyRows != nil {
		seedLegacyRows(t, db)
	}

	// a008：分区化。不全量跑 a*.sql：a001/a003/a004 依赖 chat-svc 的视图/物化视图
	// （跨服务迁移，在本 fixture 里不可用）；本测试的立论面 = user_behavior_events
	// 表本身，链路 02 → a006 → a008 与 dev 库该表的实际演化路径一致。
	applyMigration(t, db, root, "a008_partition_user_behavior_events.sql")

	for _, name := range extraMigrations {
		applyMigration(t, db, root, name)
	}
	return db, cleanup
}

// seedLegacyRows 在 a008 之前由 F-175 测试注入（非 nil 时）。
var seedLegacyRows func(t *testing.T, db *gorm.DB)

// applyMigration 执行 analytics-svc/migrations/<name>（剥 psql 元命令）。
func applyMigration(t *testing.T, db *gorm.DB, root, name string) {
	t.Helper()
	p := filepath.Join(root, "emotion-echo-analytics-svc", "migrations", name)
	require.FileExists(t, p, "找不到迁移 %s", name)
	require.NoError(t, applySQLFileStripPsqlMeta(db, p), "apply migration %s", name)
}

// repoRootFromIntegration 从 integration_test/ 出发定位仓库根（同 ai-svc repoRootFromTests）。
func repoRootFromIntegration(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err, "取 cwd 失败")
	for _, rel := range []string{"../..", "../../.."} {
		p := filepath.Join(cwd, rel)
		if _, err := os.Stat(filepath.Join(p, "deploy", "db", "01-create-schemas.sql")); err == nil {
			return p
		}
	}
	t.Fatalf("定位仓库根失败：cwd=%s 下找不到 deploy/db/01-create-schemas.sql", cwd)
	return ""
}

// applySQLFile 复用 events_integration_test.go 里的同名 helper，此处不重复定义。

// applySQLFileStripPsqlMeta 剥掉 psql 元命令行（`\set` 等，database/sql 不认）再执行。
func applySQLFileStripPsqlMeta(db *gorm.DB, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), `\`) {
			continue
		}
		kept = append(kept, line)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(strings.Join(kept, "\n"))
	if err != nil {
		return fmt.Errorf("exec %s: %w", path, err)
	}
	return nil
}

// TestUserBehaviorEvent_Create_PartitionedTable_Idempotent_Integration
//
// 分区表 schema 上 PostgresEventRepo.Create 的两条断言：
//  1. 首次写入成功（修复前此步即 42P10 RED）
//  2. 同 event_id 重复写入不报错、仍 1 行（at-least-once 消费幂等 / DLQ 回放兜底）
func TestUserBehaviorEvent_Create_PartitionedTable_Idempotent_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, cleanup := pgContainerPartitionedEvents(t, ctx)
	defer cleanup()

	// schema 自检：本测试的立论前提是表确实是分区表（否则测了个寂寞）
	var partKind string
	require.NoError(t, db.Raw(
		`SELECT partstrat FROM pg_partitioned_table pt
          JOIN pg_class c ON c.oid = pt.partrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relname = 'user_behavior_events' AND n.nspname = 'emotion_echo_analytics'`,
	).Scan(&partKind).Error)
	assert.Equal(t, "r", partKind, "user_behavior_events 必须是 RANGE 分区表，否则本测试前提不成立")

	var uqCols []string
	require.NoError(t, db.Raw(
		`SELECT a.attname FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
          JOIN unnest(con.conkey) WITH ORDINALITY k(attnum, ord) ON true
          JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
         WHERE con.contype = 'u' AND c.relname = 'user_behavior_events'
           AND n.nspname = 'emotion_echo_analytics'
         ORDER BY k.ord`,
	).Scan(&uqCols).Error)
	assert.Equal(t, []string{"event_id", "occurred_at"}, uqCols,
		"UNIQUE 约束必须是 (event_id, occurred_at)（分区表强制含分区键）")

	repo := repository.NewPostgresEventRepo(db)
	occurred := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	first := &model.UserBehaviorEvent{
		EventID:    "evt-part-dup-1",
		UserID:     42,
		EventType:  "message.created",
		Target:     "msg:1",
		SessionID:  "conv:1",
		OccurredAt: occurred,
	}
	require.NoError(t, repo.Create(ctx, first), "分区表上首次 Create 必须成功（F-149 修复前此步 42P10）")
	require.NotZero(t, first.ID, "首次插入应回填 ID")

	second := &model.UserBehaviorEvent{
		EventID:    "evt-part-dup-1", // 同 event_id（回放/重复投递场景）
		UserID:     42,
		EventType:  "message.created",
		Target:     "msg:1",
		SessionID:  "conv:1",
		OccurredAt: occurred, // 同 occurred_at（同一条事件）
	}
	require.NoError(t, repo.Create(ctx, second), "重复 event_id 必须走 ON CONFLICT DO NOTHING 而非报错")

	var count int
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM emotion_echo_analytics.user_behavior_events WHERE event_id = 'evt-part-dup-1'`,
	).Scan(&count).Error)
	assert.Equal(t, 1, count, "重复写入后必须仍是 1 行（幂等）")
}

// TestUserBehaviorEvent_PartitionSequence_AdvancesAfterPartitionSwap_Integration
//
// E2E-F-175 回归钉：a008 换表后序列必须 ≥ 老数据 max(id)。
// 复现路径：02 建表 → 显式插入 id 1..5（模拟老数据）→ a006/a008（复制+换表，
// 序列未推进）→ a010（setval）→ repo.Create 新行 ⇒ 新行 id 必须 > 5。
// 没跑 a010 时（RED）：新行 id=1，与老行空间重叠。
func TestUserBehaviorEvent_PartitionSequence_AdvancesAfterPartitionSwap_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	// 在 a008 之前造 5 行"老数据"（显式 id 1..5，模拟分区化前的存量）
	seedLegacyRows = func(t *testing.T, db *gorm.DB) {
		t.Helper()
		for i := 1; i <= 5; i++ {
			require.NoError(t, db.Exec(
				`INSERT INTO emotion_echo_analytics.user_behavior_events
				   (id, user_id, event_type, target, occurred_at, event_id)
				 VALUES (?, 42, 'message.created', ?, '2026-09-01 00:00:00+00', ?)`,
				i, fmt.Sprintf("msg:%d", i), fmt.Sprintf("evt-legacy-%d", i),
			).Error)
		}
	}
	defer func() { seedLegacyRows = nil }()

	db, cleanup := pgContainerPartitionedEvents(t, ctx, "a010_fix_partition_id_sequence.sql")
	defer cleanup()

	// 前提自检：a008 的复制数据在表里
	var legacy int
	require.NoError(t, db.Raw(`SELECT count(*) FROM emotion_echo_analytics.user_behavior_events WHERE id <= 5`).
		Scan(&legacy).Error)
	require.Equal(t, 5, legacy, "前置：老数据（id 1..5）必须已随 a008 复制进分区表")

	var seqVal int64
	require.NoError(t, db.Raw(
		`SELECT last_value FROM emotion_echo_analytics.user_behavior_events_partitioned_id_seq`,
	).Scan(&seqVal).Error)
	assert.GreaterOrEqual(t, seqVal, int64(5),
		"a010 后序列必须 ≥ 老数据 max(id)=5（F-175：a008 未推进序列，新行 id 会从 1 重发）")

	repo := repository.NewPostgresEventRepo(db)
	ev := &model.UserBehaviorEvent{
		EventID:    "evt-seq-advance-1",
		UserID:     42,
		EventType:  "message.created",
		Target:     "msg:seq",
		SessionID:  "conv:seq",
		OccurredAt: time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC),
	}
	require.NoError(t, repo.Create(ctx, ev))
	assert.Greater(t, ev.ID, int64(5),
		"新插入行 id 必须 > 老数据 max(id)=5，否则 id 游标分页/去重语义被破坏（F-175）")
}
