//go:build integration
// +build integration

// Package integration_test — runner_integration_test.go
//
// Stage 30-A SQL 落地 Part 5 RED: MentalHealthRunner 端到端集成测试
// （migration 005 assessment_jobs + 真 repo + PostgresJobStore）。
//
// 跑：go test -tags integration -v -timeout 5m ./integration_test/...
package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/trigger"
)

// mustRunnerHarness 起完整库（001-004）+ 005 + 真 repo/job store
func mustRunnerHarness(t *testing.T, ctx context.Context) (*gorm.DB, *trigger.MentalHealthRunner, func()) {
	t.Helper()
	db, cleanup := pgContainerFull(t, ctx)

	// migration 005
	p := findMigrationsFile(t, "005_create_assessment_jobs.sql")
	require.NoError(t, applySQLFile(db, p), "apply 005")

	repo := repository.NewPostgresMentalHealthRepo(db)
	store := trigger.NewPostgresJobStore(db)
	runner := trigger.NewMentalHealthRunner(repo, store)
	return db, runner, cleanup
}

type jobRowResult struct {
	Status   string
	Result   []byte
	Error    string
	HasError bool
}

func fetchJob(t *testing.T, db *gorm.DB, taskID string) (jobRowResult, bool) {
	t.Helper()
	var row struct {
		Status string
		Result []byte
		Error  *string
	}
	err := db.Raw(
		`SELECT status, result, error FROM emotion_echo_analytics.assessment_jobs WHERE task_id = $1`,
		taskID,
	).Scan(&row).Error
	require.NoError(t, err)
	if row.Status == "" {
		return jobRowResult{}, false
	}
	out := jobRowResult{Status: row.Status, Result: row.Result}
	if row.Error != nil {
		out.Error = *row.Error
		out.HasError = true
	}
	return out, true
}

func TestMentalHealthRunner_Integration_HappyPath_JobDone(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, runner, cleanup := mustRunnerHarness(t, ctx)
	defer cleanup()

	// daily 窗口内一条评估（score 55 → moderate）
	seedAssessmentWithDims(t, db, 42, "PHQ-9", 55.0, `{}`, time.Now().Add(-1*time.Hour))

	err := runner.Run(ctx, trigger.Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "it-task-1",
	})
	require.NoError(t, err)

	row, ok := fetchJob(t, db, "it-task-1")
	require.True(t, ok, "job 行应存在")
	assert.Equal(t, "done", row.Status)

	// jsonb 规范化输出（键排序 + 冒号后空格）→ 反序列化后比对字段
	var got map[string]any
	require.NoError(t, json.Unmarshal(row.Result, &got))
	assert.Equal(t, float64(55), got["overallScore"])
	assert.Equal(t, "moderate", got["riskLevel"])
	assert.Equal(t, "daily", got["type"])
	assert.False(t, row.HasError)
}

func TestMentalHealthRunner_Integration_NoData_JobDoneNull(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx := context.Background()
	db, runner, cleanup := mustRunnerHarness(t, ctx)
	defer cleanup()

	// 无评估数据
	err := runner.Run(ctx, trigger.Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "it-task-2",
	})
	require.NoError(t, err)

	row, ok := fetchJob(t, db, "it-task-2")
	require.True(t, ok)
	assert.Equal(t, "done", row.Status)
	assert.Equal(t, "null", string(row.Result), "无数据时 result 为 JSON null")
}
