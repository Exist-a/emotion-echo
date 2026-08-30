//go:build integration
// +build integration

// Package integration_test — report_repo_integration_test.go
//
// Stage 30-A SQL 落地 Part 3 RED: PostgresReportRepo 真 SQL 的
// 真实 Postgres 集成测试（daily + trend）。
//
// 依赖完整 migrations 001-004：4 个 schema + 底层表 + 跨 schema VIEWs
// + mv_daily_emotion。种子数据确定性注入。
//
// 跑：go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormpg "gorm.io/driver/postgres"
	gormlogger "gorm.io/gorm/logger"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/testcontainers/testcontainers-go"

	"emotion-echo-analytics-svc/internal/repository"
)

// pgContainerFull 起 postgres + 建 4 个 schema + 底层表 + apply migrations 001-004。
//
// 底层表列与 migration 001 VIEW 契约对齐：
//   - emotion_echo_chat.messages 用 created_at（VIEW 内 alias 为 send_time）
//   - emotion_echo_assessment.mental_health_assessments 无 risk_level
func pgContainerFull(t *testing.T, ctx context.Context) (*gorm.DB, func()) {
	t.Helper()

	pgC, err := pgcontainer.RunContainer(ctx,
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

	cleanup := func() { _ = pgC.Terminate(ctx) }

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	require.NoError(t, err)

	// 1. schemas
	for _, s := range []string{"emotion_echo_chat", "emotion_echo_ai", "emotion_echo_assessment", "emotion_echo_analytics"} {
		require.NoError(t, db.Exec("CREATE SCHEMA IF NOT EXISTS "+s).Error)
	}

	// 2. 底层表（与 migration 001 VIEW 对齐）
	require.NoError(t, db.Exec(`
CREATE TABLE emotion_echo_chat.messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    content_type VARCHAR(16) DEFAULT 'text',
    tokens_used INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE emotion_echo_ai.emotion_analysis (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    conversation_id BIGINT NOT NULL,
    primary_emotion VARCHAR(32),
    sentiment_score REAL,
    confidence REAL,
    model VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW()
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE emotion_echo_assessment.mental_health_assessments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    assessment_type VARCHAR(64),
    period_start DATE,
    period_end DATE,
    overall_score REAL,
    dimensions JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
)`).Error)

	// 3. migrations 001-004
	for _, m := range []string{
		"001_create_views.sql",
		"002_create_user_behavior_events.sql",
		"003_create_mv_daily_emotion.sql",
		"004_create_analytics_reader_role.sql",
	} {
		p := findMigrationsFile(t, m)
		require.NoError(t, applySQLFile(db, p), "apply %s", m)
	}

	return db, cleanup
}

// seedMessages 插入 messages 行（created_at 显式）
func seedMessages(t *testing.T, db *gorm.DB, userID, convID int64, n int, at time.Time) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := db.Exec(
			`INSERT INTO emotion_echo_chat.messages (conversation_id, user_id, role, content, created_at)
			 VALUES ($1, $2, 'user', $3, $4)`,
			convID, userID, "seed-content", at,
		).Error
		require.NoError(t, err)
	}
}

// seedEmotions 插入 emotion_analysis 行
func seedEmotions(t *testing.T, db *gorm.DB, userID int64, rows []struct {
	emotion   string
	sentiment float64
	conf      float64
	at        time.Time
}) {
	t.Helper()
	for i, r := range rows {
		err := db.Exec(
			`INSERT INTO emotion_echo_ai.emotion_analysis
			 (message_id, user_id, conversation_id, primary_emotion, sentiment_score, confidence, model, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 'test', $7)`,
			int64(i+1), userID, int64(i+1), r.emotion, r.sentiment, r.conf, r.at,
		).Error
		require.NoError(t, err)
	}
}

// seedAssessments 插入 mental_health_assessments 行
func seedAssessments(t *testing.T, db *gorm.DB, userID int64, rows []struct {
	atype  string
	pStart string
	pEnd   string
	score  float64
	at     time.Time
}) {
	t.Helper()
	for i, r := range rows {
		err := db.Exec(
			`INSERT INTO emotion_echo_assessment.mental_health_assessments
			 (user_id, assessment_type, period_start, period_end, overall_score, dimensions, created_at)
			 VALUES ($1, $2, $3, $4, $5, '{}', $6)`,
			userID, r.atype, r.pStart, r.pEnd, r.score, r.at,
		).Error
		require.NoError(t, err, "seed assessment %d", i)
	}
}

// mustReportRepo 构造 PostgresReportRepo（失败即停）
func mustReportRepo(t *testing.T, ctx context.Context) (*gorm.DB, repository.ReportRepo, func()) {
	t.Helper()
	db, cleanup := pgContainerFull(t, ctx)
	return db, repository.NewPostgresReportRepo(db), cleanup
}

func TestPostgresReportRepo_GetDailyReport_Aggregates_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustReportRepo(t, ctx)
	defer cleanup()

	day := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	// emotion_analysis: 2 happy + 1 sad on 07-10; 1 happy on 07-11 (excluded)
	seedEmotions(t, db, 42, []struct {
		emotion   string
		sentiment float64
		conf      float64
		at        time.Time
	}{
		{"happy", 0.8, 0.9, time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)},
		{"happy", 0.7, 0.8, time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)},
		{"sad", -0.3, 0.6, time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC)},
		{"happy", 0.9, 0.95, time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)},
	})
	// messages: 4 on 07-10, 1 on 07-11
	seedMessages(t, db, 42, 1, 4, time.Date(2026, 7, 10, 9, 30, 0, 0, time.UTC))
	seedMessages(t, db, 42, 1, 1, time.Date(2026, 7, 11, 9, 30, 0, 0, time.UTC))
	// behavior events: 2 conversations on 07-10, 1 on 07-11
	seedBehaviorEvents(t, db, 42, []struct {
		eventType string
		sessionID string
		at        time.Time
	}{
		{"conversation", "c-1", time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)},
		{"conversation", "c-2", time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)},
		{"conversation", "c-3", time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC)},
	})
	// assessments: 1 on 07-10
	seedAssessments(t, db, 42, []struct {
		atype  string
		pStart string
		pEnd   string
		score  float64
		at     time.Time
	}{
		{"weekly", "2026-07-06", "2026-07-12", 55.0, time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)},
	})

	rep, err := repo.GetDailyReport(ctx, 42, day)
	require.NoError(t, err)
	require.NotNil(t, rep)

	assert.Equal(t, int64(42), rep.UserID)
	assert.Equal(t, "2026-07-10", rep.Date)
	assert.Equal(t, map[string]int64{"happy": 2, "sad": 1}, rep.EmotionCounts)
	assert.Equal(t, int64(4), rep.MessageCount)
	assert.Equal(t, int64(2), rep.ConversationCount)
	assert.Equal(t, int64(1), rep.AssessmentCount)
	assert.InDelta(t, 0.4, rep.AvgSentiment, 1e-6)
	assert.InDelta(t, 0.7667, rep.AvgConfidence, 1e-3)
}

func TestPostgresReportRepo_GetDailyReport_EmptyDay_ZeroCounts_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustReportRepo(t, ctx)
	defer cleanup()

	// user 43 有数据，user 42 无数据 → 全 0
	seedEmotions(t, db, 43, []struct {
		emotion   string
		sentiment float64
		conf      float64
		at        time.Time
	}{
		{"happy", 0.8, 0.9, time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)},
	})

	day := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	rep, err := repo.GetDailyReport(ctx, 42, day)
	require.NoError(t, err)
	require.NotNil(t, rep)
	assert.Empty(t, rep.EmotionCounts, "无数据时 EmotionCounts 为空 map")
	assert.Equal(t, int64(0), rep.MessageCount)
	assert.Equal(t, int64(0), rep.ConversationCount)
	assert.Equal(t, int64(0), rep.AssessmentCount)
	assert.Equal(t, float64(0), rep.AvgSentiment)
	assert.Equal(t, float64(0), rep.AvgConfidence)
}

func TestPostgresReportRepo_GetTrendReport_WeeklyBuckets_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustReportRepo(t, ctx)
	defer cleanup()

	// 周 1（07-06..07-12）：happy×3 + sad×1 → primary=happy, count=4, avg=(0.8+0.7+0.6-0.3)/4=0.45
	// 周 2（07-13..07-19）：sad×2 → primary=sad, count=2, avg=-0.3
	seedEmotions(t, db, 42, []struct {
		emotion   string
		sentiment float64
		conf      float64
		at        time.Time
	}{
		{"happy", 0.8, 0.9, time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)},
		{"happy", 0.7, 0.8, time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC)},
		{"happy", 0.6, 0.7, time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)},
		{"sad", -0.3, 0.6, time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)},
		{"sad", -0.3, 0.6, time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)},
		{"sad", -0.3, 0.6, time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)},
	})

	start := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 19, 0, 0, 0, 0, time.Local)

	rep, err := repo.GetTrendReport(ctx, 42, "weekly", start, end)
	require.NoError(t, err)
	require.NotNil(t, rep)
	assert.Equal(t, int64(42), rep.UserID)
	assert.Equal(t, "weekly", rep.Type)
	assert.Len(t, rep.Points, 2, "07-06 与 07-13 两个周桶")

	p0 := rep.Points[0]
	assert.Equal(t, "2026-07-06", p0.Date)
	assert.Equal(t, "happy", p0.PrimaryEmotion)
	assert.Equal(t, int64(4), p0.Count)
	assert.InDelta(t, 0.45, p0.AvgSentiment, 1e-6)

	p1 := rep.Points[1]
	assert.Equal(t, "2026-07-13", p1.Date)
	assert.Equal(t, "sad", p1.PrimaryEmotion)
	assert.Equal(t, int64(2), p1.Count)
	assert.InDelta(t, -0.3, p1.AvgSentiment, 1e-6)
}

func TestPostgresReportRepo_GetTrendReport_ContiguousFill_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustReportRepo(t, ctx)
	defer cleanup()

	// 只有 07-08 有数据（happy×2）；窗口 07-06..07-26 → 3 个周桶，中间空桶补 0
	seedEmotions(t, db, 42, []struct {
		emotion   string
		sentiment float64
		conf      float64
		at        time.Time
	}{
		{"happy", 0.8, 0.9, time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)},
		{"happy", 0.7, 0.8, time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)},
	})

	start := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 26, 0, 0, 0, 0, time.Local)

	rep, err := repo.GetTrendReport(ctx, 42, "weekly", start, end)
	require.NoError(t, err)
	require.NotNil(t, rep)
	require.Len(t, rep.Points, 3, "07-06 / 07-13 / 07-20 三桶，中间必须补 0")

	assert.Equal(t, "2026-07-06", rep.Points[0].Date)
	assert.Equal(t, int64(2), rep.Points[0].Count)
	assert.Equal(t, "happy", rep.Points[0].PrimaryEmotion)

	assert.Equal(t, "2026-07-13", rep.Points[1].Date)
	assert.Equal(t, int64(0), rep.Points[1].Count, "空桶 Count 必须为 0")

	assert.Equal(t, "2026-07-20", rep.Points[2].Date)
	assert.Equal(t, int64(0), rep.Points[2].Count)
}
