//go:build integration
// +build integration

// Package integration_test — mentalhealth_repo_integration_test.go
//
// Stage 30-A SQL 落地 Part 4 RED: PostgresMentalHealthRepo 3 个方法的
// 真实 Postgres 集成测试（assessment / history / trend）。
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

	"emotion-echo-analytics-svc/internal/repository"
)

// seedAssessmentWithDims 插入带 dimensions JSONB 的 assessment 行
func seedAssessmentWithDims(t *testing.T, db *gorm.DB, userID int64, atype string, score float64, dims string, at time.Time) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO emotion_echo_assessment.mental_health_assessments
		 (user_id, assessment_type, period_start, period_end, overall_score, dimensions, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
		userID, atype, "2026-07-06", "2026-07-12", score, dims, at,
	).Error
	require.NoError(t, err)
}

// mustMentalHealthRepo 构造 PostgresMentalHealthRepo（失败即停）
func mustMentalHealthRepo(t *testing.T, ctx context.Context) (*gorm.DB, repository.MentalHealthRepo, func()) {
	t.Helper()
	db, cleanup := pgContainerFull(t, ctx)
	return db, repository.NewPostgresMentalHealthRepo(db), cleanup
}

func TestPostgresMentalHealthRepo_GetLatestAssessment_DailyWindow_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	now := time.Now()
	// now-1h（daily 窗口内）score 40 → moderate；now-3d（窗口外）
	seedAssessmentWithDims(t, db, 42, "PHQ-9", 40.0,
		`{"depression":{"score":45,"riskLevel":"moderate","count":2}}`, now.Add(-1*time.Hour))
	seedAssessmentWithDims(t, db, 42, "PHQ-9", 90.0, `{}`, now.Add(-72*time.Hour))

	got, err := repo.GetLatestAssessment(ctx, 42, repository.AssessmentDaily)
	require.NoError(t, err)
	require.NotNil(t, got, "daily 窗口内应有评估")
	assert.Equal(t, int64(42), got.UserID)
	assert.Equal(t, "daily", got.Type)
	assert.Equal(t, float64(40), got.OverallScore)
	assert.Equal(t, "moderate", got.RiskLevel, "risk 由 overall_score 推导")
	require.Len(t, got.Dimensions, 1, "dimensions JSONB 应映射为 DimensionScore")
	assert.Equal(t, "depression", got.Dimensions[0].Name)
	assert.Equal(t, 45.0, got.Dimensions[0].Score)
	assert.Equal(t, "moderate", got.Dimensions[0].RiskLevel)
	assert.Equal(t, 2, got.Dimensions[0].Count)
	assert.Equal(t, "2026-07-06", got.WindowStart)
	assert.Equal(t, "2026-07-12", got.WindowEnd)
}

func TestPostgresMentalHealthRepo_GetLatestAssessment_DailyEmpty_NilNil_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	now := time.Now()
	// 只有 3 天前的行 → daily（24h 窗口）无结果 → (nil, nil)
	seedAssessmentWithDims(t, db, 42, "PHQ-9", 90.0, `{}`, now.Add(-72*time.Hour))

	got, err := repo.GetLatestAssessment(ctx, 42, repository.AssessmentDaily)
	require.NoError(t, err)
	assert.Nil(t, got, "窗口外无评估时返回 (nil, nil)，不返 error")
}

func TestPostgresMentalHealthRepo_GetLatestAssessment_WeeklyAndComprehensive_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	now := time.Now()
	seedAssessmentWithDims(t, db, 42, "PHQ-9", 60.0, `{}`, now.Add(-72*time.Hour))  // 3d
	seedAssessmentWithDims(t, db, 42, "GAD-7", 85.0, `{}`, now.Add(-20*24*time.Hour)) // 20d

	weekly, err := repo.GetLatestAssessment(ctx, 42, repository.AssessmentWeekly)
	require.NoError(t, err)
	require.NotNil(t, weekly)
	assert.Equal(t, float64(60), weekly.OverallScore, "weekly 窗口 7 天 → 取 3d 行")

	comprehensive, err := repo.GetLatestAssessment(ctx, 42, repository.AssessmentComprehensive)
	require.NoError(t, err)
	require.NotNil(t, comprehensive)
	assert.Equal(t, float64(60), comprehensive.OverallScore, "comprehensive 取全部中的最新（3d > 20d）")
}

func TestPostgresMentalHealthRepo_ListAssessmentHistory_Pagination_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	// 5 行（created_at 递增 → id 递增）
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		seedAssessments(t, db, 42, []struct {
			atype  string
			pStart string
			pEnd   string
			score  float64
			at     time.Time
		}{
			{"PHQ-9", "2026-07-01", "2026-07-07", float64(i * 10), base.Add(time.Duration(i) * 24 * time.Hour)},
		})
	}

	// 第 1 页：limit=2 → id 5,4，nextCursor="4"
	items, next, err := repo.ListAssessmentHistory(ctx, 42, "", "", 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, uint64(5), items[0].ID)
	assert.Equal(t, uint64(4), items[1].ID)
	assert.Equal(t, "4", next, "还有更多 → nextCursor 是最后一条 id")

	// 第 2 页：cursor="4" → id 3,2，nextCursor="2"
	items, next, err = repo.ListAssessmentHistory(ctx, 42, "", "4", 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, uint64(3), items[0].ID)
	assert.Equal(t, uint64(2), items[1].ID)
	assert.Equal(t, "2", next)

	// 第 3 页：cursor="2" → id 1，nextCursor=""
	items, next, err = repo.ListAssessmentHistory(ctx, 42, "", "2", 2)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, uint64(1), items[0].ID)
	assert.Equal(t, "", next, "没有更多 → nextCursor 空")

	// 字段映射
	first := items[0]
	assert.Equal(t, int64(42), first.UserID)
	assert.Equal(t, "PHQ-9", first.AssessmentType)
	assert.Equal(t, float64(10), first.OverallScore)
	assert.Equal(t, "low", first.RiskLevel)
	assert.Equal(t, "2026-07-01", first.PeriodStart)
	assert.Equal(t, "2026-07-07", first.PeriodEnd)
}

func TestPostgresMentalHealthRepo_ListAssessmentHistory_TypeFilter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i, atype := range []string{"PHQ-9", "GAD-7", "PHQ-9"} {
		seedAssessments(t, db, 42, []struct {
			atype  string
			pStart string
			pEnd   string
			score  float64
			at     time.Time
		}{
			{atype, "2026-07-01", "2026-07-07", float64((i + 1) * 10), base.Add(time.Duration(i+1) * 24 * time.Hour)},
		})
	}

	items, next, err := repo.ListAssessmentHistory(ctx, 42, "PHQ-9", "", 10)
	require.NoError(t, err)
	require.Len(t, items, 2, "只有 2 条 PHQ-9")
	assert.Equal(t, "PHQ-9", items[0].AssessmentType)
	assert.Equal(t, "PHQ-9", items[1].AssessmentType)
	assert.Equal(t, "", next)
}

func TestPostgresMentalHealthRepo_GetTrendData_WeeklyBuckets_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	// 07-08 score 50（周 1）；07-15 score 80（周 2）→ 连续桶 07-06 / 07-13
	seedAssessments(t, db, 42, []struct {
		atype  string
		pStart string
		pEnd   string
		score  float64
		at     time.Time
	}{
		{"PHQ-9", "2026-07-06", "2026-07-12", 50.0, time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)},
		{"PHQ-9", "2026-07-13", "2026-07-19", 80.0, time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)},
	})

	start := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 19, 0, 0, 0, 0, time.Local)

	points, err := repo.GetTrendData(ctx, 42, "weekly", start, end)
	require.NoError(t, err)
	require.Len(t, points, 2, "07-06 / 07-13 两个周桶")

	assert.Equal(t, "2026-07-06", points[0].Date)
	assert.Equal(t, int64(1), points[0].Count)
	assert.Equal(t, 50.0, points[0].AvgSentiment)
	assert.Equal(t, "moderate", points[0].PrimaryEmotion, "score 50 → risk moderate")

	assert.Equal(t, "2026-07-13", points[1].Date)
	assert.Equal(t, int64(1), points[1].Count)
	assert.Equal(t, 80.0, points[1].AvgSentiment)
	assert.Equal(t, "severe", points[1].PrimaryEmotion, "score 80 → risk severe")
}

func TestPostgresMentalHealthRepo_GetTrendData_EmptyGapFilled_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, repo, cleanup := mustMentalHealthRepo(t, ctx)
	defer cleanup()

	// 只有 07-08（周 1）；窗口 07-06..07-26 → 3 桶，07-13 / 07-20 空桶 Count=0
	seedAssessments(t, db, 42, []struct {
		atype  string
		pStart string
		pEnd   string
		score  float64
		at     time.Time
	}{
		{"PHQ-9", "2026-07-06", "2026-07-12", 30.0, time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)},
	})

	start := time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 26, 0, 0, 0, 0, time.Local)

	points, err := repo.GetTrendData(ctx, 42, "weekly", start, end)
	require.NoError(t, err)
	require.Len(t, points, 3)
	assert.Equal(t, int64(1), points[0].Count)
	assert.Equal(t, int64(0), points[1].Count)
	assert.Equal(t, int64(0), points[2].Count)
}
