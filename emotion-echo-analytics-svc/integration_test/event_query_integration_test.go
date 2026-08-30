//go:build integration
// +build integration

// Package integration_test — event_query_integration_test.go
//
// Stage 30-A SQL 落地 Part 1 RED: PostgresEventRepo 3 个查询方法的
// 真实 Postgres 集成测试（day-night / interaction-depth / frequency）。
//
// 依赖：
//   - testcontainers postgres:15-alpine
//   - migration 002 (user_behavior_events)
//
// 跑：go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"emotion-echo-analytics-svc/internal/repository"
)

// seedBehaviorEvents 用 raw SQL 插入一批 user_behavior_events（显式时间戳，确定性）。
func seedBehaviorEvents(t *testing.T, db *gorm.DB, userID int64, rows []struct {
	eventType string
	sessionID string
	at        time.Time
}) {
	t.Helper()
	for i, r := range rows {
		err := db.Exec(
			`INSERT INTO emotion_echo_analytics.user_behavior_events
			 (user_id, event_type, target, session_id, occurred_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, r.eventType, fmt.Sprintf("tgt-%d", i), r.sessionID, r.at,
		).Error
		require.NoError(t, err, "seed event %d failed", i)
	}
}

// mustEventRepo 构造 PostgresEventRepo + 原始 db（用于种子数据；失败即停）
func mustEventRepo(t *testing.T, ctx context.Context) (*gorm.DB, repository.EventRepo, func()) {
	t.Helper()
	db, cleanup := pgContainerForEvents(t, ctx)
	return db, repository.NewPostgresEventRepo(db), cleanup
}

func TestPostgresEventRepo_GetDayNightPattern_HourBuckets_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustEventRepo(t, ctx)
	defer cleanup()

	// 2026-07-10 有 08:00 ×2 / 14:00 ×3 / 23:00 ×1；07-11 08:00 ×1（窗口外 day2 应被 end 排掉）
	rows := []struct {
		eventType string
		sessionID string
		at        time.Time
	}{
		{"message", "s-a", time.Date(2026, 7, 10, 8, 5, 0, 0, time.UTC)},
		{"message", "s-a", time.Date(2026, 7, 10, 8, 40, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 10, 14, 15, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 10, 14, 55, 0, 0, time.UTC)},
		{"message", "s-c", time.Date(2026, 7, 10, 23, 30, 0, 0, time.UTC)},
		// end=07-10 → 07-11 的行必须在窗口外
		{"message", "s-d", time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC)},
	}
	seedBehaviorEvents(t, db, 42, rows)

	start := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	got, err := repo.GetDayNightPattern(ctx, 42, start, end)
	require.NoError(t, err)
	assert.Equal(t, map[int]int64{8: 2, 14: 3, 23: 1}, got, "hour 桶应正确聚合且排除窗口外行")
}

func TestPostgresEventRepo_GetDayNightPattern_OtherUserIsolated_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustEventRepo(t, ctx)
	defer cleanup()

	rows := []struct {
		eventType string
		sessionID string
		at        time.Time
	}{
		{"message", "s-a", time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)},
	}
	seedBehaviorEvents(t, db, 1, rows)
	// 另一个 user 查询 → 空 map
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)

	got, err := repo.GetDayNightPattern(ctx, 999, start, end)
	require.NoError(t, err)
	assert.Empty(t, got, "其他 user 不应看到任何桶")
}

func TestPostgresEventRepo_GetInteractionDepth_TotalsAndLongest_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustEventRepo(t, ctx)
	defer cleanup()

	// session s-a: 3 条，跨度 30 分钟（08:00 → 08:30）= 1_800_000 ms
	// session s-b: 2 条，跨度 10 分钟（10:00 → 10:10）= 600_000 ms
	// total=5, conversations=2, avg=2.5, longest=1_800_000
	rows := []struct {
		eventType string
		sessionID string
		at        time.Time
	}{
		{"message", "s-a", time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)},
		{"message", "s-a", time.Date(2026, 7, 10, 8, 15, 0, 0, time.UTC)},
		{"message", "s-a", time.Date(2026, 7, 10, 8, 30, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 10, 10, 10, 0, 0, time.UTC)},
	}
	seedBehaviorEvents(t, db, 42, rows)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)

	depth, err := repo.GetInteractionDepth(ctx, 42, start, end)
	require.NoError(t, err)
	require.NotNil(t, depth)
	assert.Equal(t, int64(5), depth.TotalMessages)
	assert.Equal(t, int64(2), depth.TotalConversations)
	assert.Equal(t, 2.5, depth.AvgMessagesPerConv)
	assert.Equal(t, int64(1_800_000), depth.LongestConversationMs)
}

func TestPostgresEventRepo_GetInteractionDepth_NoSessions_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	_, repo, cleanup := mustEventRepo(t, ctx)
	defer cleanup()

	// 窗口内无事件 → 全 0，不 panic（除零保护）
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)

	depth, err := repo.GetInteractionDepth(ctx, 42, start, end)
	require.NoError(t, err)
	require.NotNil(t, depth)
	assert.Equal(t, int64(0), depth.TotalMessages)
	assert.Equal(t, int64(0), depth.TotalConversations)
	assert.Equal(t, float64(0), depth.AvgMessagesPerConv)
	assert.Equal(t, int64(0), depth.LongestConversationMs)
}

func TestPostgresEventRepo_GetFrequencyTrend_DailyBuckets_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustEventRepo(t, ctx)
	defer cleanup()

	// 07-08 ×2、07-09 ×3、07-10 ×4；07-11 ×1（窗口外）
	rows := []struct {
		eventType string
		sessionID string
		at        time.Time
	}{
		{"message", "s-a", time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)},
		{"message", "s-a", time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 9, 11, 0, 0, 0, time.UTC)},
		{"message", "s-b", time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)},
		{"message", "s-c", time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)},
		{"message", "s-c", time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC)},
		{"message", "s-c", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)},
		{"message", "s-c", time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)},
		{"message", "s-d", time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)},
	}
	seedBehaviorEvents(t, db, 42, rows)

	start := time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local)

	got, err := repo.GetFrequencyTrend(ctx, 42, start, end)
	require.NoError(t, err)
	require.Len(t, got, 3, "应返回 3 个有数据的日期桶")
	assert.Equal(t, "2026-07-08", got[0].Date)
	assert.Equal(t, int64(2), got[0].Count)
	assert.Equal(t, "2026-07-09", got[1].Date)
	assert.Equal(t, int64(3), got[1].Count)
	assert.Equal(t, "2026-07-10", got[2].Date)
	assert.Equal(t, int64(4), got[2].Count)
}

func TestPostgresEventRepo_GetFrequencyTrend_EmptyWindow_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	_, repo, cleanup := mustEventRepo(t, ctx)
	defer cleanup()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)

	got, err := repo.GetFrequencyTrend(ctx, 42, start, end)
	require.NoError(t, err)
	assert.Empty(t, got, "无数据时返回空 slice 而非 nil")
}
